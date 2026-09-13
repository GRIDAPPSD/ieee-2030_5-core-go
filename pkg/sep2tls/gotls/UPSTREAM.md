# Upstream provenance: pkg/sep2tls/gotls

`pkg/sep2tls/gotls` is a fork of the Go standard library's `crypto/tls`,
carrying the mandatory IEEE 2030.5 / CSIP CCM-8 cipher suite that upstream
does not implement. This file records what it was forked from, what changed,
how to reproduce the diff, and how to check upstream security fixes against
it. It describes the tree at this branch only: it makes no statement about
the effect of any pull request that has not merged.

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
`crypto/tls` (the `FILES` list in `check-upstream.sh`), the fork's copy was
rewritten to swap `package gotls` back to `package tls` and the four local
stub import paths back to their real standard-library paths (see
"Deliberate changes" below), then passed through `gofmt`, then diffed
against each candidate release's copy of the same file.

Diff line counts below are the added-plus-removed content lines `diff -u`
reports, not counting the `---`/`+++` file-header line pair it prints once
per differing file. Given a raw `diff -u` run saved to `out`, the exact
command is:

```
grep -Ec '^[+-][^+-]' out
```

| Candidate | Commit | Diff lines | Files differing |
|---|---|---|---|
| go1.21.13 (latest Go 1.21.x) | `8bba868de983dd7bf55fcd121495ba8d6e2734e7` | 209 | 10 of 20 |
| **go1.22.0** | `a10e42f219abb9c5bc4e7d86d9464700a42c7d57` | **0** | 0 of 20 |
| go1.22.12 (latest Go 1.22.x) | `5817e650946aaa0ac28956de96b3f9aa1de4b299` | 4 | 2 of 20 |

Go 1.22.0 is a byte-for-byte match, after `gofmt`, on every one of the 20
shared files.

Before trusting the zero: the same procedure run against go1.21.13 produced
209 changed lines and a nonzero exit. Both tags carry the same 22 non-test
files under `src/crypto/tls` (confirmed by listing both checkouts); the
difference is in file content, not file layout. The check can fail; it did,
against the wrong base.

go1.22.12 is not a byte-for-byte match: it differs from go1.22.0 by 4 lines
in `handshake_client.go` and `handshake_server.go`, where upstream added an
`!needFIPS() &&` guard around a debug counter increment
(`tlsrsakex.IncNonDefault()`) between go1.22.0 and go1.22.12. This is a real,
small upstream content change, not an import-ordering artifact, and it does
not disappear under `gofmt`. It has no effect on the fork's behavior, because
the fork's `needFIPS` always returns `false` (see `notboring.go`), so the
guarded call always runs regardless of the guard's presence. Go 1.22.0
remains the fork's exact, byte-for-byte base; later 1.22.x point releases are
not implied to match it and were not all checked (only go1.22.12, the latest,
was).

## Deliberate changes against the base

| Change | Files | Notes |
|---|---|---|
| Package rename `tls` -> `gotls` | all 20 non-test `.go` files carried from upstream | Required so the fork can be vendored as an ordinary importable package rather than the standard library's `crypto/tls`. |
| Internal package stand-ins | new files `stubs/boring/boring.go`, `stubs/cpu/cpu.go`, `stubs/fipstls/fipstls.go`, `stubs/godebug/godebug.go`; import sites: `stubs/fipstls` in `boring.go`; `stubs/boring` and `stubs/cpu` in `cipher_suites.go`; `stubs/godebug` in `common.go`, `conn.go`, and `handshake_client.go` | Upstream `crypto/tls` imports four packages under `internal/` or `crypto/internal/`, which are not importable outside the standard library: `crypto/internal/boring`, `crypto/internal/boring/fipstls`, `internal/cpu`, `internal/godebug`. Each is replaced by a local stub package of the same name and function signatures, with `Enabled`/`Required`/`HasAES` and friends hardcoded to their disabled/false values and `Setting.Value()` hardcoded to `""`. `notboring.go` is otherwise unmodified upstream code; it already assumes `boring.Enabled == false` at compile time via a build tag pair, which this stand-in preserves logically without reproducing the build-tag split. |
| CCM-8 cipher suite registration | new file `cipher_suites_ccm.go` | Registers `TLS_ECDHE_ECDSA_WITH_AES_128_CCM_8` (`0xC0AE`, RFC 7251), the suite IEEE 2030.5-2018 SEP2 mandates, via an `init()` function that appends to `cipherSuites`, `cipherSuitesPreferenceOrder`, and `cipherSuitesPreferenceOrderNoAES`. It does not edit `cipher_suites.go` itself, which is why that file is byte-identical to upstream. The AEAD construction calls the sibling `pkg/sep2tls/ccm` package (a vendored pure-Go RFC 3610 CCM implementation), not a standard-library CCM. |
| Fork-only tests | new files `ccm_check_test.go`, `ccm_raw_test.go` | Assert the CCM-8 suite is registered and exercise a raw handshake using it. Not part of upstream and not part of the diff. |
| Dropped files | `generate_cert.go`, `fipsonly/fipsonly.go` (both upstream `crypto/tls`) | `generate_cert.go` is a `package main`, `//go:build ignore` standalone certificate-generation tool, not part of the `tls` package's compiled surface. `fipsonly/fipsonly.go` is a separate package (`crypto/tls/fipsonly`) that, when blank-imported, forces FIPS-only TLS configuration; it only exists under `GOEXPERIMENT=boringcrypto` and calls `crypto/internal/boring/fipstls` and `crypto/internal/boring/sig` directly, neither of which the fork stands in for. Neither file is carried into the fork. |

## Known divergences (not yet reconciled with upstream)

- **Legacy ClientHello handling**: `handshake_server.go` matches go1.22.0
  byte-for-byte (see the base comparison above), and go1.22.0's
  `handshake_server.go` does not contain the RFC 8446 Section 4.2.1 branch that
  rejects a pre-TLS-1.3-style ClientHello carrying `legacy_version =
  0x0304` with no `supported_versions` extension. That branch is present at
  go1.25.0 and go1.27.1, and absent at go1.22.0, go1.22.12, go1.23.0,
  go1.23.12, and go1.24.0 (checked directly against each tag's
  `handshake_server.go`). Whether it was backported to a later go1.24.x
  point release is not checked. Since the fork has never been rebased past
  go1.22.0, it does not have this change either way.
- **No AES-NI / hardware AES detection for cipher suite preference**:
  `stubs/cpu/cpu.go` hardcodes `HasAES` (and the other feature flags) to
  `false` on every architecture. `cipher_suites.go` reads this once, to
  compute `hasAESGCMHardwareSupport`, which selects between
  `cipherSuitesPreferenceOrder` and `cipherSuitesPreferenceOrderNoAES` in
  `handshake_client.go` and `handshake_server.go`. So the fork always
  negotiates as if no hardware AES acceleration were present, which can
  affect which cipher suite a handshake settles on. It does not affect
  whether AES itself runs in hardware: the AES primitives come from the
  standard library's `crypto/aes.NewCipher`, which does its own,
  independent CPU feature detection at the point the cipher is constructed.
  This is a property of the stand-in design, not an upstream-fix gap, and
  it holds regardless of which upstream base the fork is rebased to.

## Reproducing the diff and checking for unrecorded files

`check-upstream.sh`, in this directory, is the repeatable check. It needs
network access to `github.com/golang/go` (a read-only mirror of
`go.googlesource.com/go`, bounded to a 90-second clone) and `git`, `gofmt`,
`sed`, `diff`, `sha256sum`, `find`, `awk`, `cut`, `mktemp`, and `timeout` on
`PATH`. Run it from the repository root (the directory containing this
project's `go.mod`):

```
pkg/sep2tls/gotls/check-upstream.sh
```

It does three things:

1. Diffs the 20 shared files (normalized back to upstream package and
   import names, then `gofmt`-formatted) against the recorded upstream tag,
   the same comparison used to establish the base above.
2. Checks every file listed in `upstream-manifest.sha256` against its
   recorded sha256. That file covers the fork-only files that have no
   upstream counterpart (`cipher_suites_ccm.go`, `ccm_check_test.go`,
   `ccm_raw_test.go`, and the four `stubs/*/*.go` files) and any shared file
   that has been deliberately patched (see "Recording a deliberate change"
   below). A content change to any of these with no matching manifest
   update is reported.
3. Scans every file or symlink under `pkg/sep2tls/gotls` and reports any
   path that is neither one of the 20 shared files, a manifest entry, nor the check
   script, its `bats` test suite, the manifest, or this document. A new
   file added to the tree without being classified one way or the other is
   reported rather than passing silently.

Exit codes:

| Exit | Meaning |
|---|---|
| 0 | Clean: every shared file matches upstream (or a recorded patch), every manifest entry matches its recorded hash, no unrecorded file. |
| 2 | Environment: a required tool is missing, the script was not run from the repository root, or the upstream clone/checkout could not be produced as recorded (bad tag, commit mismatch, sparse-checkout failure, network failure, an upstream file absent after checkout). This is a tooling or setup failure, not evidence of drift, and is never combined with the bits below: it always ends the run by itself. |
| any other nonzero | A bitwise OR of: **1** (drift: a shared file differs from upstream with no recorded patch, a manifest entry's hash no longer matches its recorded content, or an expected shared file is missing) and **4** (unrecorded: a file or symlink exists under `pkg/sep2tls/gotls` that this script cannot classify; add it to the `FILES` list in `check-upstream.sh` if it is meant to track an upstream file, or record it in `upstream-manifest.sha256` if it is fork-only). So exit 5 means both drift and an unrecorded file were found in the same run; neither overwrites the other. |

A nonzero exit prints one or more messages on stderr naming which check
failed and why; read the message to tell drift, an environment failure,
and an unrecorded file apart, since each calls for a different fix, and a
combined exit code (5) means more than one message is present.

## Recording a deliberate change

Two situations call for a manifest update rather than a code change:

- **A new fork-only file** (for example, a new stub or a new fork-only
  test): add a `fork-only` line to `upstream-manifest.sha256` with
  `sha256sum pkg/sep2tls/gotls/<path>` and a short note of why the file
  exists. Without this, `check-upstream.sh` reports it as unrecorded (exit
  3).
- **A hand-ported fix to a shared file** (see the security-release
  procedure below): after making the change, add or update a `patched`
  line in `upstream-manifest.sha256` for that file, again with its current
  `sha256sum` and a note naming the release or advisory the fix came from.
  Once a shared file has a `patched` entry, `check-upstream.sh` stops
  diffing it against upstream and instead checks its hash against that
  entry, so the check passes on the recorded change and still fails if the
  file changes again without the manifest being updated.

## Checking upstream `crypto/tls` security releases against the fork

1. Watch the Go security announcements
   (`https://groups.google.com/g/golang-announce`) or the release notes at
   `https://go.dev/doc/devel/release` for any release whose fix list
   mentions `crypto/tls`.
2. For each such release, fetch the affected file(s) from
   `https://go.googlesource.com/go/+/refs/tags/<tag>/src/crypto/tls/<file>`
   (append `?format=TEXT` for a base64 body, or temporarily edit
   `UPSTREAM_TAG` and `UPSTREAM_COMMIT` in a scratch copy of
   `check-upstream.sh` to the fixed release and run it against the current
   base's checkout, to see the security delta directly) and compare the
   affected function against the fork's copy of the same file.
3. If the fix applies to a file the fork carries unmodified from go1.22.0,
   port the fix by hand into the fork file, add a new row to the
   "Deliberate changes" table above naming the CVE or release and the
   file(s) touched, and record the change per "Recording a deliberate
   change" above. Do not bump `UPSTREAM_TAG` in `check-upstream.sh` unless
   every shared file has been re-verified against the new base by the same
   procedure used to establish go1.22.0 above; a partial rebase would make
   the recorded base a false claim about files that were not actually
   re-diffed.
4. If the fix applies to `generate_cert.go`, `fipsonly/fipsonly.go`, or any
   other file the fork does not carry, no action is needed here; note it
   was checked and found inapplicable.
