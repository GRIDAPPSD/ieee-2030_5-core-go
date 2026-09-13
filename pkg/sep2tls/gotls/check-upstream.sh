#!/usr/bin/env bash
# Repeatable diff of pkg/sep2tls/gotls against its recorded upstream base,
# plus a manifest check for files that have no upstream counterpart. See
# UPSTREAM.md for what a clean run proves and how to record a deliberate
# change. Run from the repository root (the directory containing go.mod).
#
# Exit codes:
#   0  clean: every shared file matches upstream (or a recorded patch),
#      every manifest entry matches its recorded hash, and no unrecorded
#      file was found under pkg/sep2tls/gotls.
#   2  environment: a required tool is missing, the script was not run
#      from the repository root, the upstream clone/checkout could not be
#      produced as recorded (bad tag, commit mismatch, sparse-checkout
#      failure, network failure, an upstream file absent after checkout),
#      or a step this script depends on (the file walk, the normalization
#      pass) could not be completed. This is a tooling failure, not
#      evidence of drift. 2 is reserved for these and is never combined
#      with a bit below: it is returned directly, ending the run.
#   Any other nonzero exit is a bitwise OR of the codes below, so a run
#   that hits more than one condition reports all of them at once instead
#   of the last one silently winning. Read stderr for which fired.
#     1  drift: a shared file differs from upstream with no recorded
#        patch, a manifest entry's hash no longer matches the file's
#        content, or a file this script expects is missing.
#     4  unrecorded: a file (or symlink) exists under pkg/sep2tls/gotls
#        that is neither a shared file, a manifest entry, nor an ignored
#        path. Add it to FILES or record it in upstream-manifest.sha256.
#   So 5 (1|4) means both drift and an unrecorded file were found in the
#   same run.
set -euo pipefail

UPSTREAM_TAG="go1.22.0"
UPSTREAM_COMMIT="a10e42f219abb9c5bc4e7d86d9464700a42c7d57"
FORK_DIR_REL="pkg/sep2tls/gotls"
FORK_DIR="$(pwd)/${FORK_DIR_REL}"
MANIFEST="${FORK_DIR}/upstream-manifest.sha256"

# Wall-clock bound on the upstream clone. Without this an unreachable
# proxy or a stalled connection hides behind git's own retry backoff for
# minutes with no output, and callers timing the check see it as a hang
# rather than the environment failure it is.
CLONE_TIMEOUT_SECS=90

# Files shared between the fork and upstream crypto/tls. Excludes the
# fork's own additions (cipher_suites_ccm.go, ccm_*_test.go, stubs/**),
# upstream's generate_cert.go and fipsonly/fipsonly.go (standalone tools
# the fork drops), and all _test.go files (the fork carries none of
# upstream's crypto/tls tests).
FILES=(
  alert.go auth.go boring.go cache.go cipher_suites.go common.go
  common_string.go conn.go handshake_client.go handshake_client_tls13.go
  handshake_messages.go handshake_server.go handshake_server_tls13.go
  key_agreement.go key_schedule.go notboring.go prf.go quic.go ticket.go
  tls.go
)

# Paths under FORK_DIR that are check machinery or documentation, not fork
# content, so they are excluded from the unrecorded-file scan.
IGNORED=(check-upstream.sh check-upstream.bats upstream-manifest.sha256 UPSTREAM.md)

in_array() {
  local needle="$1" straw
  shift
  for straw in "$@"; do
    [ "$straw" = "$needle" ] && return 0
  done
  return 1
}

require_tools() {
  local t
  for t in git gofmt sed diff sha256sum find awk cut mktemp timeout; do
    command -v "$t" >/dev/null 2>&1 || {
      echo "error: required tool '$t' not found on PATH" >&2
      exit 2
    }
  done
}

# manifest_type PATH prints the recorded type ("fork-only" or "patched")
# for PATH, or nothing if PATH has no manifest entry.
manifest_type() {
  awk -F'\t' -v p="$1" '$1 !~ /^#/ && $2 == p { print $1; exit }' "$MANIFEST"
}

# check_manifest verifies every manifest entry's file exists and its
# sha256 still matches. Prints one "drift:" line per mismatch to stderr
# and returns 1 if any mismatch was found, 0 otherwise.
check_manifest() {
  local rc=0 type path expected note full actual
  [ -f "$MANIFEST" ] || {
    echo "error: manifest not found at $MANIFEST" >&2
    exit 2
  }
  while IFS=$'\t' read -r type path expected note; do
    [ -z "$type" ] && continue
    [[ "$type" == \#* ]] && continue
    full="$FORK_DIR/$path"
    if [ ! -f "$full" ]; then
      echo "drift: manifest entry '$path' ($note) is missing from the fork" >&2
      rc=1
      continue
    fi
    actual="$(sha256sum "$full" | cut -d' ' -f1)"
    if [ "$actual" != "$expected" ]; then
      echo "drift: $path content changed and its manifest hash was not updated ($note)" >&2
      rc=1
    fi
  done <"$MANIFEST"
  return "$rc"
}

# scan_for_unrecorded walks every file or symlink under FORK_DIR and
# reports any path that is neither a shared FILES entry, a manifest entry,
# nor an ignored path. Returns 1 if it found one, 0 otherwise; exits 2
# directly if the walk itself could not be completed (for example a find
# that rejects -printf, a GNU extension not available on BSD/macOS find),
# so a broken walk fails the run instead of silently reporting that it
# found nothing.
scan_for_unrecorded() {
  local rc=0 relpath list
  list="$(mktemp)"
  if ! (cd "$FORK_DIR" && find . \( -type f -o -type l \) -printf '%P\0') >"$list"; then
    rm -f "$list"
    echo "error: could not walk $FORK_DIR (find failed)" >&2
    exit 2
  fi
  while IFS= read -r -d '' relpath; do
    in_array "$relpath" "${IGNORED[@]}" && continue
    in_array "$relpath" "${FILES[@]}" && continue
    if [ -n "$(manifest_type "$relpath")" ]; then
      continue
    fi
    echo "unrecorded: $relpath is not in FILES or $MANIFEST" >&2
    rc=1
  done <"$list"
  rm -f "$list"
  return "$rc"
}

# diff_shared_files clones the recorded upstream tag, normalizes the
# fork's shared files back to upstream package and import names, and
# diffs each one. Skips any file recorded as "patched" in the manifest,
# since that file has a deliberate, recorded divergence and is verified
# by check_manifest instead. Returns 1 if any unpatched shared file
# differs or is missing, 0 otherwise; exits 2 directly on an environment
# failure.
diff_shared_files() {
  local work norm rc=0 f upstream_file
  work="$(mktemp -d)"
  trap 'rm -rf "$work"' EXIT INT TERM

  local clone_err
  clone_err="$(mktemp)"
  if ! timeout "$CLONE_TIMEOUT_SECS" git clone --quiet --filter=blob:none --sparse \
    --branch "$UPSTREAM_TAG" --depth 1 \
    https://github.com/golang/go "$work/go" >/dev/null 2>"$clone_err"; then
    echo "error: could not clone golang/go at tag $UPSTREAM_TAG within ${CLONE_TIMEOUT_SECS}s (network or tag failure):" >&2
    cat "$clone_err" >&2
    rm -f "$clone_err"
    exit 2
  fi
  rm -f "$clone_err"
  if ! git -C "$work/go" sparse-checkout set src/crypto/tls >/dev/null 2>&1; then
    echo "error: sparse-checkout of src/crypto/tls failed" >&2
    exit 2
  fi

  local got
  got="$(git -C "$work/go" rev-parse HEAD)"
  if [ "$got" != "$UPSTREAM_COMMIT" ]; then
    echo "error: upstream commit mismatch: got $got, want $UPSTREAM_COMMIT" >&2
    exit 2
  fi

  norm="$work/norm"
  mkdir -p "$norm" || {
    echo "error: could not create working directory $norm" >&2
    exit 2
  }

  for f in "${FILES[@]}"; do
    [ "$(manifest_type "$f")" = "patched" ] && continue

    upstream_file="$work/go/src/crypto/tls/$f"
    if [ ! -f "$upstream_file" ]; then
      echo "error: upstream file src/crypto/tls/$f not found after checkout" >&2
      exit 2
    fi
    if [ ! -f "$FORK_DIR/$f" ]; then
      echo "drift: $f is listed as shared but is missing from the fork" >&2
      rc=1
      continue
    fi
    if ! sed -e 's/^package gotls$/package tls/' \
      -e 's#github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2tls/gotls/stubs/fipstls#crypto/internal/boring/fipstls#' \
      -e 's#github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2tls/gotls/stubs/boring#crypto/internal/boring#' \
      -e 's#github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2tls/gotls/stubs/cpu#internal/cpu#' \
      -e 's#github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2tls/gotls/stubs/godebug#internal/godebug#' \
      "$FORK_DIR/$f" >"$norm/$f"; then
      echo "error: could not normalize $f for comparison (sed failed)" >&2
      exit 2
    fi
  done
  # gofmt failing here (a missing binary is already caught by
  # require_tools; this covers gofmt erroring on a specific file) leaves
  # that file un-gofmt-ed, which can only ADD a spurious diff below, not
  # hide a real one, so it fails toward reporting rather than toward
  # silence and is safe to ignore.
  gofmt -w "$norm"/*.go 2>/dev/null || true

  for f in "${FILES[@]}"; do
    [ "$(manifest_type "$f")" = "patched" ] && continue
    if [ ! -f "$norm/$f" ]; then
      echo "error: internal: normalized copy of $f not found (this is a script bug, not missing input)" >&2
      exit 2
    fi
    diff -u "$work/go/src/crypto/tls/$f" "$norm/$f" || rc=1
  done

  rm -rf "$work"
  trap - EXIT INT TERM
  return "$rc"
}

main() {
  require_tools

  if [ ! -f "$(pwd)/go.mod" ]; then
    echo "error: run from the repository root (the directory containing go.mod)" >&2
    exit 2
  fi
  if [ ! -f "$MANIFEST" ]; then
    echo "error: manifest not found at $MANIFEST" >&2
    exit 2
  fi

  local status=0
  diff_shared_files || status=$((status | 1))
  check_manifest || status=$((status | 1))
  scan_for_unrecorded || status=$((status | 4))

  exit "$status"
}

main "$@"
