#!/usr/bin/env bats
# Regression coverage for check-lint-gate.sh's own control flow: package
# discovery, the canary's pass/fail decisions, and the workflow-weakening
# checks. `golangci-lint` and `go` are stubbed (shell.md: mock external
# commands) so these tests run offline and do not invoke the real tools.

setup() {
  SCRIPT="$BATS_TEST_DIRNAME/check-lint-gate.sh"
  WORKDIR="$(mktemp -d)"
  cd "$WORKDIR" || exit 1

  git init -q
  git config user.email test@example.invalid
  git config user.name test

  mkdir -p pkg/one pkg/sep2tls/gotls pkg/sep2tls/ccm .github/workflows bin

  echo 'package gotls' >pkg/sep2tls/gotls/foo.go
  echo 'package ccm' >pkg/sep2tls/ccm/bar.go
  echo 'package one' >pkg/one/foo.go

  cat >.golangci.yml <<'YAML'
version: "2"
linters:
  exclusions:
    paths:
      - ^pkg/sep2tls/gotls/
      - ^pkg/sep2tls/ccm/
YAML

  cat >.github/workflows/ci.yml <<'YAML'
name: CI
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo build
  lint:
    runs-on: ubuntu-latest
    steps:
      - name: Lint
        uses: golangci/golangci-lint-action@v9
        with:
          version: v2.12.2
YAML

  git add -A

  # go is stubbed only for "go list", which discover_packages runs to
  # find first-party package directories.
  cat >bin/go <<'STUB'
#!/usr/bin/env bash
if [ "$1" = "list" ]; then
  printf '%s\n' "${STUB_GO_LIST_OUTPUT-$PWD/pkg/one|one}"
  exit "${STUB_GO_LIST_EXIT:-0}"
fi
exit 1
STUB
  chmod +x bin/go

  cat >bin/golangci-lint <<'STUB'
#!/usr/bin/env bash
if [ "$1" = "version" ]; then
  printf 'golangci-lint has version stub\n'
  exit 0
fi
if [ -n "${STUB_LINT_ARGV_FILE:-}" ]; then
  printf '%s\n' "$*" >>"$STUB_LINT_ARGV_FILE"
fi
printf '%s' "${STUB_LINT_OUTPUT:-}"
exit "${STUB_LINT_EXIT:-1}"
STUB
  chmod +x bin/golangci-lint
  # A curated PATH, not the inherited one: golangci-lint may be installed
  # for the operator outside the standard system directories (for example
  # under a home directory), and prepending the stub to the full
  # inherited PATH would let that real binary satisfy `command -v` the
  # moment a test removes bin/golangci-lint, defeating the PATH-absence
  # test below.
  PATH="$WORKDIR/bin:/usr/local/bin:/usr/bin:/bin"

  # Default: a healthy canary outcome for the single discovered package
  # pkg/one, plus no fork markers, and golangci-lint's own exit code of 1
  # (issues found, no tool error).
  export STUB_LINT_OUTPUT="pkg/one/zz_lint_canary.go:8:6: func lintCanaryUnused is unused (unused)
pkg/one/zz_lint_canary.go:11:11: Error return value of \`os.Remove\` is not checked (errcheck)
pkg/one/zz_lint_canary.go:15:14: fmt.Printf format %d has arg \"x\" of wrong type string (govet)
pkg/one/zz_lint_canary.go:19:2: ineffectual assignment to x (ineffassign)
pkg/one/zz_lint_canary.go:26:9: S1002: should omit comparison to bool constant, can be simplified to b (staticcheck)
pkg/one/zz_lint_canary_test.go:6:11: Error return value of \`os.Remove\` is not checked (errcheck)"
  export STUB_LINT_EXIT=1
}

teardown() {
  cd /
  rm -rf "$WORKDIR"
}

@test "passes when every planted first-party pair is reported, no fork pair is, and exit is exactly 1" {
  run bash "$SCRIPT"
  [ "$status" -eq 0 ]
  [[ "$output" == *"lint gate check passed"* ]]
}

@test "fails when one linter's pair is missing in a package" {
  # Each @test runs in its own bats subshell by design; "run" reads this
  # export in that same subshell, so the value is not actually lost.
  # shellcheck disable=SC2030,SC2031
  export STUB_LINT_OUTPUT="pkg/one/zz_lint_canary.go:8:6: func lintCanaryUnused is unused (unused)
pkg/one/zz_lint_canary.go:15:14: fmt.Printf format %d has arg \"x\" of wrong type string (govet)
pkg/one/zz_lint_canary.go:19:2: ineffectual assignment to x (ineffassign)
pkg/one/zz_lint_canary.go:26:9: S1002: should omit comparison to bool constant, can be simplified to b (staticcheck)
pkg/one/zz_lint_canary_test.go:6:11: Error return value of \`os.Remove\` is not checked (errcheck)"
  run bash "$SCRIPT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"planted violation not reported: errcheck in pkg/one/zz_lint_canary.go"* ]]
}

@test "fails when the test-file pair is missing" {
  # Each @test runs in its own bats subshell by design; "run" reads this
  # export in that same subshell, so the value is not actually lost.
  # shellcheck disable=SC2030,SC2031
  export STUB_LINT_OUTPUT="pkg/one/zz_lint_canary.go:8:6: func lintCanaryUnused is unused (unused)
pkg/one/zz_lint_canary.go:11:11: Error return value of \`os.Remove\` is not checked (errcheck)
pkg/one/zz_lint_canary.go:15:14: fmt.Printf format %d has arg \"x\" of wrong type string (govet)
pkg/one/zz_lint_canary.go:19:2: ineffectual assignment to x (ineffassign)
pkg/one/zz_lint_canary.go:26:9: S1002: should omit comparison to bool constant, can be simplified to b (staticcheck)"
  run bash "$SCRIPT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"planted violation not reported: errcheck in pkg/one/zz_lint_canary_test.go"* ]]
}

@test "fails when a fork plant is reported" {
  # Each @test runs in its own bats subshell by design; "run" reads this
  # export in that same subshell, so the value is not actually lost.
  # shellcheck disable=SC2030,SC2031
  export STUB_LINT_OUTPUT="$STUB_LINT_OUTPUT
pkg/sep2tls/gotls/zz_lint_canary.go:8:6: func lintCanaryGotlsUnused is unused (unused)"
  run bash "$SCRIPT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"vendored fork file was reported by golangci-lint: pkg/sep2tls/gotls/zz_lint_canary.go"* ]]
}

@test "fails when a fork plant is not written" {
  # Simulate the fork directory disappearing between checkout and the
  # canary run (a rename or deletion the check must not silently accept).
  # git rm removes it from both the working tree and the index, so
  # build_scratch_tree's git-ls-files copy never recreates the directory.
  git rm -rqf pkg/sep2tls/ccm
  run bash "$SCRIPT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"expected fork directory not found in scratch tree: pkg/sep2tls/ccm"* ]]
}

@test "fails when golangci-lint exits 0" {
  # Each @test runs in its own bats subshell by design; "run" reads this
  # export in that same subshell, so the value is not actually lost.
  # shellcheck disable=SC2030,SC2031
  export STUB_LINT_EXIT=0
  run bash "$SCRIPT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"exited 0 against the canary tree"* ]]
}

@test "fails when golangci-lint exits 3 with every pair present" {
  # Each @test runs in its own bats subshell by design; "run" reads this
  # export in that same subshell, so the value is not actually lost.
  # shellcheck disable=SC2030,SC2031
  export STUB_LINT_EXIT=3
  run bash "$SCRIPT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"exited 3 against the canary tree"* ]]
}

@test "fails when a level=warning line appears with every pair present" {
  # Each @test runs in its own bats subshell by design; "run" reads this
  # export in that same subshell, so the value is not actually lost.
  # shellcheck disable=SC2030,SC2031
  export STUB_LINT_OUTPUT="$STUB_LINT_OUTPUT
level=warning msg=\"[runner] Can't process results by diff processor: can't prepare diff by revgrep: no version control repository found\""
  run bash "$SCRIPT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"printed a level=warning or level=error line"* ]]
}

@test "fails closed when go list discovers zero first-party packages" {
  export STUB_GO_LIST_OUTPUT=""
  run bash "$SCRIPT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"reported 0 first-party packages"* ]]
}

@test "fails closed when golangci-lint is absent from PATH" {
  rm -f bin/golangci-lint
  run bash "$SCRIPT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"golangci-lint is not on PATH"* ]]
}

@test "a planted file name present only outside an issue line does not count as reported" {
  # "pkg/one/zz_lint_canary.go" appears in the message body of a finding
  # anchored on a different file, so it must not satisfy the expected
  # pair for pkg/one/zz_lint_canary.go's own errcheck plant.
  # Each @test runs in its own bats subshell by design; "run" reads this
  # export in that same subshell, so the value is not actually lost.
  # shellcheck disable=SC2030,SC2031
  export STUB_LINT_OUTPUT="pkg/one/zz_lint_canary.go:8:6: func lintCanaryUnused is unused (unused)
pkg/one/other.go:1:1: mentions pkg/one/zz_lint_canary.go in the message (errcheck)
pkg/one/zz_lint_canary.go:15:14: fmt.Printf format %d has arg \"x\" of wrong type string (govet)
pkg/one/zz_lint_canary.go:19:2: ineffectual assignment to x (ineffassign)
pkg/one/zz_lint_canary.go:26:9: S1002: should omit comparison to bool constant, can be simplified to b (staticcheck)
pkg/one/zz_lint_canary_test.go:6:11: Error return value of \`os.Remove\` is not checked (errcheck)"
  run bash "$SCRIPT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"planted violation not reported: errcheck in pkg/one/zz_lint_canary.go"* ]]
}

@test "fails closed when git ls-files fails while building the canary tree" {
  rm -rf .git
  run bash "$SCRIPT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"git ls-files failed"* ]]
}

@test "fails closed when .golangci.yml is missing" {
  rm .golangci.yml
  run bash "$SCRIPT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"required file not found: .golangci.yml"* ]]
}

@test "fails closed when the ci workflow has no lint job" {
  cat >.github/workflows/ci.yml <<'YAML'
name: CI
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo build
YAML
  run bash "$SCRIPT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"no 'lint:' job found"* ]]
}

@test "fails when the lint job sets continue-on-error: true" {
  sed -i '/uses: golangci\/golangci-lint-action@v9/a\        continue-on-error: true' .github/workflows/ci.yml
  run bash "$SCRIPT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"sets continue-on-error to true"* ]]
}

@test "fails when the lint job sets continue-on-error: True" {
  sed -i '/uses: golangci\/golangci-lint-action@v9/a\        continue-on-error: True' .github/workflows/ci.yml
  run bash "$SCRIPT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"sets continue-on-error to true"* ]]
}

@test "fails when the lint job sets continue-on-error: \${{ true }}" {
  sed -i '/uses: golangci\/golangci-lint-action@v9/a\        continue-on-error: ${{ true }}' .github/workflows/ci.yml
  run bash "$SCRIPT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"sets continue-on-error to true"* ]]
}

@test "fails when the Lint step has if: false" {
  sed -i '/- name: Lint/a\        if: false' .github/workflows/ci.yml
  run bash "$SCRIPT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"if: false"* ]]
}

@test "fails when the lint job sets only-new-issues: true" {
  sed -i '/version: v2.12.2/a\          only-new-issues: true' .github/workflows/ci.yml
  run bash "$SCRIPT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"only-new-issues: true"* ]]
}

@test "fails when the lint job overrides issues-exit-code via args" {
  sed -i '/version: v2.12.2/a\          args: --issues-exit-code=0' .github/workflows/ci.yml
  run bash "$SCRIPT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"overrides issues-exit-code to 0"* ]]
}

@test "fails when the golangci-lint-action step is removed from the lint job" {
  cat >.github/workflows/ci.yml <<'YAML'
name: CI
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo build
  lint:
    runs-on: ubuntu-latest
    steps:
      - run: echo lint
YAML
  run bash "$SCRIPT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"no golangci-lint-action step found"* ]]
}
