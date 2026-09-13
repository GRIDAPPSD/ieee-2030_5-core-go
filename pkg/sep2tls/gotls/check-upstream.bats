#!/usr/bin/env bats
# Smoke test for check-upstream.sh: the happy path plus one test per
# documented failure path (missing tool, wrong cwd, missing manifest,
# clone/commit failure, content drift, and an unrecorded file). The
# upstream clone is mocked (see mock_git below) so the suite needs no
# network access and is not sensitive to real upstream content changes.

setup() {
  SRC_DIR="$BATS_TEST_DIRNAME"
  WORK="$(mktemp -d)"
  REPO="$WORK/repo"
  FORK_DIR="$REPO/pkg/sep2tls/gotls"

  mkdir -p "$REPO/pkg/sep2tls"
  : >"$REPO/go.mod"
  cp -r "$SRC_DIR" "$FORK_DIR"

  # A frozen "upstream" fixture, built once from the pristine fork copy
  # above, before any test mutates it. This is what the mocked git
  # presents as the recorded upstream tag, so the happy path diffs clean
  # by construction and a later mutation to $FORK_DIR shows up as drift
  # against this fixture, not against a moving target.
  UPSTREAM_FIXTURE="$WORK/upstream-fixture/src/crypto/tls"
  mkdir -p "$UPSTREAM_FIXTURE"
  for f in "$FORK_DIR"/*.go; do
    base="$(basename "$f")"
    case "$base" in
    cipher_suites_ccm.go | ccm_check_test.go | ccm_raw_test.go) continue ;;
    esac
    sed -e 's/^package gotls$/package tls/' \
      -e 's#github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2tls/gotls/stubs/fipstls#crypto/internal/boring/fipstls#' \
      -e 's#github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2tls/gotls/stubs/boring#crypto/internal/boring#' \
      -e 's#github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2tls/gotls/stubs/cpu#internal/cpu#' \
      -e 's#github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2tls/gotls/stubs/godebug#internal/godebug#' \
      "$f" >"$UPSTREAM_FIXTURE/$base"
  done
  gofmt -w "$UPSTREAM_FIXTURE"/*.go

  MOCKBIN="$WORK/mockbin"
  mkdir -p "$MOCKBIN"
  cat >"$MOCKBIN/git" <<MOCKEOF
#!/usr/bin/env bash
set -euo pipefail
if [ "\$1" = "clone" ]; then
  if [ -n "\${MOCK_GIT_FAIL_CLONE:-}" ]; then
    echo "mock: clone failed" >&2
    exit 1
  fi
  dest="\${!#}"
  mkdir -p "\$dest"
  exit 0
fi
if [ "\$1" = "-C" ]; then
  dir="\$2"
  sub="\$3"
  if [ "\$sub" = "sparse-checkout" ]; then
    mkdir -p "\$dir/src/crypto/tls"
    cp "$UPSTREAM_FIXTURE"/*.go "\$dir/src/crypto/tls/"
    exit 0
  fi
  if [ "\$sub" = "rev-parse" ]; then
    echo "\${MOCK_GIT_COMMIT:-a10e42f219abb9c5bc4e7d86d9464700a42c7d57}"
    exit 0
  fi
fi
echo "mock git: unhandled args: \$*" >&2
exit 1
MOCKEOF
  chmod +x "$MOCKBIN/git"

  PATH="$MOCKBIN:$PATH"
  export PATH REPO FORK_DIR WORK
}

teardown() {
  rm -rf "$WORK"
}

run_check() {
  (cd "$REPO" && bash "$FORK_DIR/check-upstream.sh")
}

@test "clean tree against the recorded upstream fixture exits 0" {
  run run_check
  [ "$status" -eq 0 ]
}

@test "missing required tool exits 2" {
  # A curated PATH holding every external tool check-upstream.sh itself
  # calls, resolved to a real executable path with "type -P" (so a
  # builtin or a shell function with no backing file is never
  # symlinked), except git: git is left out, so "command -v git" fails
  # and the script's own tool-presence guard is what's under test.
  local no_git_bin="$WORK/no-git-bin"
  mkdir -p "$no_git_bin"
  for t in bash sed diff sha256sum gofmt mkdir rm mktemp find awk cut timeout; do
    ln -s "$(type -P "$t")" "$no_git_bin/$t"
  done
  PATH="$no_git_bin" run run_check
  [ "$status" -eq 2 ]
  [[ "$output" == *"required tool 'git'"* ]]
}

@test "running outside the repository root exits 2" {
  run bash -c "cd '$WORK' && bash '$FORK_DIR/check-upstream.sh'"
  [ "$status" -eq 2 ]
  [[ "$output" == *"repository root"* ]]
}

@test "a missing manifest exits 2" {
  rm "$FORK_DIR/upstream-manifest.sha256"
  run run_check
  [ "$status" -eq 2 ]
  [[ "$output" == *"manifest not found"* ]]
}

@test "an upstream clone failure exits 2, not 1" {
  export MOCK_GIT_FAIL_CLONE=1
  run run_check
  [ "$status" -eq 2 ]
}

@test "an upstream commit mismatch exits 2, not 1" {
  export MOCK_GIT_COMMIT=deadbeef
  run run_check
  [ "$status" -eq 2 ]
}

@test "a shared file that drifts from the fixture exits 1" {
  printf '\n// test-only marker\n' >>"$FORK_DIR/alert.go"
  run run_check
  [ "$status" -eq 1 ]
}

@test "an edited fork-only file exits 1 via the manifest check" {
  sed -i 's/keyLen: 16,/keyLen: 32,/' "$FORK_DIR/cipher_suites_ccm.go"
  run run_check
  [ "$status" -eq 1 ]
  [[ "$output" == *"drift: cipher_suites_ccm.go"* ]]
}

@test "an unrecorded new file exits 4" {
  printf 'package gotls\n' >"$FORK_DIR/unrecorded.go"
  run run_check
  [ "$status" -eq 4 ]
  [[ "$output" == *"unrecorded: unrecorded.go"* ]]
}

@test "a symlink under FORK_DIR is scanned, not skipped by -type f" {
  ln -s alert.go "$FORK_DIR/evil-link.go"
  run run_check
  [ "$status" -eq 4 ]
  [[ "$output" == *"unrecorded: evil-link.go"* ]]
}

@test "drift and an unrecorded file combine into exit 5 instead of one overwriting the other" {
  printf '\n// test-only marker\n' >>"$FORK_DIR/alert.go"
  printf 'package gotls\n' >"$FORK_DIR/unrecorded.go"
  run run_check
  [ "$status" -eq 5 ]
  [[ "$output" == *"test-only marker"* ]]
  [[ "$output" == *"unrecorded: unrecorded.go"* ]]
}

@test "a malformed manifest line (missing field) exits 1 with a distinct message" {
  printf 'fork-only\tstubs/godebug/godebug.go\n' >>"$FORK_DIR/upstream-manifest.sha256"
  run run_check
  [ "$status" -eq 1 ]
  [[ "$output" == *"malformed manifest line"* ]]
}

@test "a manifest with no final newline still hash-checks its last entry" {
  local last_path
  last_path="$(tail -1 "$FORK_DIR/upstream-manifest.sha256" | cut -f2)"
  printf '\n// test-only marker\n' >>"$FORK_DIR/$last_path"
  # Strip the manifest's own trailing newline: $(...) drops it, printf
  # writes the content back with none, so the last line (the one just
  # corrupted) is what `read`'s EOF-without-newline behavior is tested
  # against.
  printf '%s' "$(cat "$FORK_DIR/upstream-manifest.sha256")" >"$FORK_DIR/upstream-manifest.sha256"
  run run_check
  [ "$status" -eq 1 ]
  [[ "$output" == *"drift: $last_path content changed"* ]]
}

@test "a wrongly hashed entry appended with no final newline still hash-checks" {
  printf 'package gotls\n// new fork-only file\n' >"$FORK_DIR/no_newline_new.go"
  printf 'fork-only\tno_newline_new.go\tdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef\t-\ttest: wrong hash, no trailing newline' \
    >>"$FORK_DIR/upstream-manifest.sha256"
  run run_check
  [ "$status" -eq 1 ]
  [[ "$output" == *"drift: no_newline_new.go content changed"* ]]
}

@test "a patched file whose recorded upstream hash still matches upstream exits 0" {
  local fork_hash upstream_hash
  fork_hash="$(sha256sum "$FORK_DIR/handshake_server.go" | cut -d' ' -f1)"
  upstream_hash="$(sha256sum "$UPSTREAM_FIXTURE/handshake_server.go" | cut -d' ' -f1)"
  printf 'patched\thandshake_server.go\t%s\t%s\ttest: pretend deliberate patch\n' \
    "$fork_hash" "$upstream_hash" >>"$FORK_DIR/upstream-manifest.sha256"
  run run_check
  [ "$status" -eq 0 ]
}

@test "a patched file reports drift once upstream moves past its recorded base hash" {
  local fork_hash
  fork_hash="$(sha256sum "$FORK_DIR/handshake_server.go" | cut -d' ' -f1)"
  printf 'patched\thandshake_server.go\t%s\tdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef\ttest: pretend deliberate patch\n' \
    "$fork_hash" >>"$FORK_DIR/upstream-manifest.sha256"
  run run_check
  [ "$status" -eq 1 ]
  [[ "$output" == *"drift: upstream handshake_server.go changed since the patch was recorded"* ]]
}

@test "a patched entry with no recorded upstream_sha256 exits 2" {
  local fork_hash
  fork_hash="$(sha256sum "$FORK_DIR/handshake_server.go" | cut -d' ' -f1)"
  printf 'patched\thandshake_server.go\t%s\t-\ttest: pretend deliberate patch\n' \
    "$fork_hash" >>"$FORK_DIR/upstream-manifest.sha256"
  run run_check
  [ "$status" -eq 2 ]
  [[ "$output" == *"no recorded upstream_sha256"* ]]
}

@test "each of find, awk, cut, mktemp, and timeout is checked before use" {
  local missing no_tool_bin t
  for missing in find awk cut mktemp timeout; do
    no_tool_bin="$WORK/no-$missing-bin"
    mkdir -p "$no_tool_bin"
    for t in bash sed diff sha256sum gofmt mkdir rm mktemp find awk cut timeout git; do
      [ "$t" = "$missing" ] && continue
      ln -sf "$(type -P "$t")" "$no_tool_bin/$t"
    done
    PATH="$no_tool_bin" run run_check
    [ "$status" -eq 2 ]
    [[ "$output" == *"required tool '$missing'"* ]]
  done
}

@test "a find that rejects -printf fails the walk instead of silently finding nothing" {
  local bsdfind_bin="$WORK/bsdfind-bin"
  mkdir -p "$bsdfind_bin"
  cat >"$bsdfind_bin/find" <<'BSDEOF'
#!/usr/bin/env bash
for a in "$@"; do
  if [ "$a" = "-printf" ]; then
    echo "find: -printf: unknown primary or operator" >&2
    exit 1
  fi
done
exec /usr/bin/find "$@"
BSDEOF
  chmod +x "$bsdfind_bin/find"
  printf 'package gotls\n' >"$FORK_DIR/unrecorded_m1.go"
  PATH="$bsdfind_bin:$PATH" run run_check
  [ "$status" -eq 2 ]
  [[ "$output" == *"could not walk"* ]]
}

@test "a failing awk reports an environment failure, not an unrecorded file" {
  local broken_awk_bin="$WORK/broken-awk-bin"
  mkdir -p "$broken_awk_bin"
  cat >"$broken_awk_bin/awk" <<'AWKEOF'
#!/usr/bin/env bash
echo "mock: awk is broken" >&2
exit 1
AWKEOF
  chmod +x "$broken_awk_bin/awk"
  PATH="$broken_awk_bin:$PATH" run run_check
  [ "$status" -eq 2 ]
  [[ "$output" == *"awk failed"* ]]
}
