# Upstream provenance: pkg/sep2tls/gotls

`pkg/sep2tls/gotls` is a fork of the Go standard library's `crypto/tls`,
carrying the mandatory IEEE 2030.5 / CSIP CCM-8 cipher suite that upstream
does not implement. This file records what it was forked from, what changed,
how to reproduce the diff, and how to check upstream security fixes against
it.

## Upstream base

- Release: **Go 1.22.0**
- Tag: `go1.22.0`
- Commit: `a10e42f219abb9c5bc4e7d86d9464700a42c7d57`
- Tag commit date (from `go.googlesource.com/go`): 2024-02-06

### How the base was identified

The fork carries no version marker, so the base was found by content match
rather than by record. The candidate window was narrowed from the file set
itself before any byte comparison ran:

- The fork has `cache.go` and `quic.go`, both absent before Go 1.21, so the
  base is Go 1.21 or later.
- The fork has neither `defaults.go` nor `ech.go`, both added in Go 1.23
  (encrypted ClientHello support), so the base predates Go 1.23.

That left the Go 1.21.x and Go 1.22.x lines as candidates. Predicted before
comparing: Go 1.22.x, on the reasoning that a fork adding a single cipher
suite is more likely cut from a stable minor release than rebased file by
file across a whole later cycle.

Comparison method: for each of the 20 files the fork shares with upstream
`crypto/tls` (see the file list in the diff command below), the fork's copy
was rewritten to swap `package gotls` back to `package tls` and the four
local stub import paths back to their real standard-library paths (see
"Deliberate changes" below), then passed through `gofmt`, then diffed
against each candidate release's copy of the same file.

Diff line counts (added + removed lines, summed across all 20 shared files,
after `gofmt`):

| Candidate | Commit | Diff lines |
|---|---|---|
| go1.21.13 (latest Go 1.21.x) | `8bba868de983dd7bf55fcd121495ba8d6e2734e7` | 209 |
| **go1.22.0** | `a10e42f219abb9c5bc4e7d86d9464700a42c7d57` | **0** |
| go1.22.12 (latest Go 1.22.x) | (see `git ls-remote --tags` on `go.googlesource.com/go`) | 8, all import-block reordering that disappears under `gofmt` (see below) |

Go 1.22.0 is a byte-for-byte match, after `gofmt`, on every one of the 20
shared files. This confirmed the prediction. The result also shows the fork
predates every later Go 1.22.x point release: none of them changed these
files relative to go1.22.0, since go1.22.12 differs from go1.22.0 only by
import-block order, an artifact of `gofmt` re-sorting the import list once
the stub paths are swapped back to their original (shorter) names, not a
real content change. `gofmt -w` on the rewritten fork copy removes that
artifact entirely (verified: 0 lines differ after formatting for every file
that showed a nonzero raw `diff` before formatting).

Before trusting the zero: the same procedure run against go1.21.13 (a
release known NOT to match, since it lacks the file layout Go 1.22.0 has)
produced 209 changed lines and a nonzero exit from the diff command below.
The check can fail; it did, against the wrong base.

## Deliberate changes against the base

| Change | Files | Notes |
|---|---|---|
| Package rename `tls` -> `gotls` | all 21 non-test `.go` files carried from upstream | Required so the fork can be vendored as an ordinary importable package rather than the standard library's `crypto/tls`. |
| Internal package stand-ins | `boring.go`, `cipher_suites.go`, `handshake_client.go` (import sites); new files `stubs/boring/boring.go`, `stubs/cpu/cpu.go`, `stubs/fipstls/fipstls.go`, `stubs/godebug/godebug.go` | Upstream `crypto/tls` imports four packages under `internal/` or `crypto/internal/`, which are not importable outside the standard library: `crypto/internal/boring`, `crypto/internal/boring/fipstls`, `internal/cpu`, `internal/godebug`. Each is replaced by a local stub package of the same name and function signatures, with `Enabled`/`Required`/`HasAES` and friends hardcoded to their disabled/false values and `Setting.Value()` hardcoded to `""`. `notboring.go` is otherwise unmodified upstream code; it already assumes `boring.Enabled == false` at compile time via a build tag pair, which this stand-in preserves logically without reproducing the build-tag split. |
| CCM-8 cipher suite registration | new file `cipher_suites_ccm.go` | Registers `TLS_ECDHE_ECDSA_WITH_AES_128_CCM_8` (`0xC0AE`, RFC 7251), the suite IEEE 2030.5-2018 SEP2 mandates, via an `init()` function that appends to `cipherSuites`, `cipherSuitesPreferenceOrder`, and `cipherSuitesPreferenceOrderNoAES`. It does not edit `cipher_suites.go` itself, which is why that file is byte-identical to upstream. The AEAD construction calls the sibling `pkg/sep2tls/ccm` package (a vendored pure-Go RFC 3610 CCM implementation), not a standard-library CCM. |
| Fork-only tests | new files `ccm_check_test.go`, `ccm_raw_test.go` | Assert the CCM-8 suite is registered and exercise a raw handshake using it. Not part of upstream and not part of the diff below. |
| Dropped file | `generate_cert.go` (upstream `crypto/tls`) | A `package main`, `//go:build ignore` standalone certificate-generation tool, not part of the `tls` package's compiled surface. Not carried into the fork. |

## Known divergences (not yet reconciled with upstream; unverified by this record)

These are reported findings, not independently confirmed by this record.
Source: `artifacts/outputs/leon-tls-dual-version-feasibility-2026-09-13.md`,
findings LOW-5 and LOW-6, in the workspace at
`/home/debian/repos/ieee-2030_5-project`.

- **LOW-5, legacy ClientHello handling**: the fork is reported to lack an
  upstream fix for old-style TLS 1.3 ClientHellos that carry
  `legacy_version = 0x0304` with no `supported_versions` extension. If real,
  this is not a fork-introduced defect: `handshake_server.go` matches
  go1.22.0 byte-for-byte (see the diff above), so any such fix would have to
  have landed in a Go release after go1.22.0, and the fork has never been
  rebased past it. Unverified by this record: confirm against the CVE or
  commit that introduced the fix, then check whether it postdates go1.22.0.
- **LOW-6, no AES-NI / hardware AES detection**: `stubs/cpu/cpu.go` hardcodes
  `HasAES = false` (and the other feature flags) on every architecture, so
  the fork always takes the pure-Go AES path even on hardware that has
  AES-NI, ARMv8 crypto extensions, or the S390x equivalents. Confirmed by
  reading `stubs/cpu/cpu.go`: this is exactly what the stub does, unlike
  LOW-5 this is not an upstream-fix gap, it is a property of the stand-in
  design and holds regardless of the upstream base chosen.

## Pending change: GRIDAPPSD/ieee-2030_5-core-go#136

A change in progress on another branch (not merged as of this writing)
reorders `cipherSuitesPreferenceOrder` and
`cipherSuitesPreferenceOrderNoAES` in `cipher_suites_ccm.go` so CCM-8 is
preferred ahead of the GCM suites already in those lists, rather than
appended after them. That branch is out of scope for this record: this file
is not edited by it and should be re-read, not assumed current, once #136
lands, since the change list above for `cipher_suites_ccm.go` will need a
line for it.

## Reproducing the diff

The command below clones a sparse, shallow copy of the upstream Go source
tree at the recorded tag, rewrites the fork's package name and stub import
paths back to their upstream form, runs `gofmt`, and diffs each shared file.
It needs network access to `github.com/golang/go` (a read-only mirror of
`go.googlesource.com/go`) and a `git`, `gofmt`, `sed`, `diff` on `PATH`.

Save the block below as a script (for example `/tmp/gotls-diff.sh`) and run
it from the repository root (the directory containing this project's
`go.mod`):

```bash
#!/usr/bin/env bash
# Repeatable diff of pkg/sep2tls/gotls against its recorded upstream base.
# Run from the repository root (the directory containing go.mod).
set -euo pipefail

UPSTREAM_TAG="go1.22.0"
UPSTREAM_COMMIT="a10e42f219abb9c5bc4e7d86d9464700a42c7d57"
FORK_DIR="$(pwd)/pkg/sep2tls/gotls"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

git clone --quiet --filter=blob:none --sparse --branch "$UPSTREAM_TAG" --depth 1 \
  https://github.com/golang/go "$WORK/go" >/dev/null
git -C "$WORK/go" sparse-checkout set src/crypto/tls >/dev/null
got="$(git -C "$WORK/go" rev-parse HEAD)"
if [ "$got" != "$UPSTREAM_COMMIT" ]; then
  echo "upstream commit mismatch: got $got, want $UPSTREAM_COMMIT" >&2
  exit 1
fi

NORM="$WORK/norm"
mkdir -p "$NORM"

# Files shared between the fork and upstream crypto/tls. Excludes the fork's
# own additions (cipher_suites_ccm.go, ccm_*_test.go, stubs/**), upstream's
# generate_cert.go (a standalone cmd the fork drops), and all _test.go files
# (the fork carries none of upstream's).
FILES="alert.go auth.go boring.go cache.go cipher_suites.go common.go
common_string.go conn.go handshake_client.go handshake_client_tls13.go
handshake_messages.go handshake_server.go handshake_server_tls13.go
key_agreement.go key_schedule.go notboring.go prf.go quic.go ticket.go tls.go"

for f in $FILES; do
  sed -e 's/^package gotls$/package tls/' \
      -e 's#github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2tls/gotls/stubs/fipstls#crypto/internal/boring/fipstls#' \
      -e 's#github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2tls/gotls/stubs/boring#crypto/internal/boring#' \
      -e 's#github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2tls/gotls/stubs/cpu#internal/cpu#' \
      -e 's#github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2tls/gotls/stubs/godebug#internal/godebug#' \
      "$FORK_DIR/$f" > "$NORM/$f"
done
gofmt -w "$NORM"/*.go

status=0
for f in $FILES; do
  diff -u "$WORK/go/src/crypto/tls/$f" "$NORM/$f" || status=1
done
exit $status
```

A clean run produces no diff output and exits 0. Any output means either a
deliberate change was made outside the list above (update this file) or
upstream drift needs reconciling (see the next section).

## Checking upstream `crypto/tls` security releases against the fork

1. Watch the Go security announcements
   (`https://groups.google.com/g/golang-announce`) or the release notes at
   `https://go.dev/doc/devel/release` for any release whose fix list
   mentions `crypto/tls`.
2. For each such release, fetch the affected file(s) from
   `https://go.googlesource.com/go/+/refs/tags/<tag>/src/crypto/tls/<file>`
   (append `?format=TEXT` for a base64 body, or use the diff command above
   with `UPSTREAM_TAG` and `UPSTREAM_COMMIT` temporarily set to the fixed
   release, run against the CURRENT base tag's checkout, to see the
   security delta directly) and compare the affected function against the
   fork's copy of the same file.
3. If the fix applies to a file the fork carries unmodified from go1.22.0,
   port the fix by hand into the fork file, and update the "Deliberate
   changes" table above with a new row naming the CVE or release and the
   file(s) touched. Do not bump `UPSTREAM_TAG` in the diff command unless
   every file has been re-verified against the new base by the same
   procedure used to establish go1.22.0 above; a partial rebase would make
   the recorded base a false claim about files that were not actually
   re-diffed.
4. If the fix applies to `generate_cert.go` or any other file the fork does
   not carry, no action is needed here; note it was checked and found
   inapplicable.
