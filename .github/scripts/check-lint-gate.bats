#!/usr/bin/env bats
# Regression coverage for check-lint-gate.sh's own control flow: the
# canary's pass/fail decisions and the workflow-weakening checks. `go` is
# stubbed (shell.md: mock external commands) so these tests run offline
# and do not build or invoke a real golangci-lint.

setup() {
  SCRIPT="$BATS_TEST_DIRNAME/check-lint-gate.sh"
  WORKDIR="$(mktemp -d)"
  cd "$WORKDIR" || exit 1

  git init -q
  git config user.email test@example.invalid
  git config user.name test

  mkdir -p pkg/sep2tls/gotls pkg/sep2tls/ccm pkg/sep2cert .github/workflows bin

  echo 'package gotls' >pkg/sep2tls/gotls/foo.go
  echo 'package ccm' >pkg/sep2tls/ccm/bar.go
  echo 'package sep2tls' >pkg/sep2tls/ccmserver.go
  echo 'package sep2cert' >pkg/sep2cert/foo.go

  cat >.golangci.yml <<'YAML'
version: "2"
run:
  build-tags: []
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

  cat >bin/go <<'STUB'
#!/usr/bin/env bash
if [ -n "${STUB_GO_ARGV_FILE:-}" ]; then
  printf '%s\n' "$*" >>"$STUB_GO_ARGV_FILE"
fi
printf '%s' "${STUB_GO_OUTPUT:-}"
exit "${STUB_GO_EXIT:-0}"
STUB
  chmod +x bin/go
  PATH="$WORKDIR/bin:$PATH"

  # Default: the healthy canary outcome (both first-party markers
  # reported, no fork markers, non-zero exit).
  export STUB_GO_OUTPUT="pkg/sep2tls/ccmserver.go:1:1: func lintCanaryCcmserverUnused is unused (unused)
pkg/sep2cert/zz_lint_canary.go:1:1: func lintCanarySep2certUnused is unused (unused)"
  export STUB_GO_EXIT=1
}

teardown() {
  cd /
  rm -rf "$WORKDIR"
}

@test "passes when golangci-lint reports both first-party canaries, no fork canaries, and exits non-zero" {
  run bash "$SCRIPT"
  [ "$status" -eq 0 ]
  [[ "$output" == *"lint gate check passed"* ]]
}

@test "fails when golangci-lint exits 0 despite the planted first-party violations" {
  export STUB_GO_EXIT=0
  run bash "$SCRIPT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"exited 0 against the canary tree"* ]]
}

@test "fails when the ccmserver.go canary is not reported" {
  # Each @test runs in its own bats subshell by design; "run" reads this
  # export in that same subshell, so the value is not actually lost.
  # shellcheck disable=SC2030,SC2031
  export STUB_GO_OUTPUT="pkg/sep2cert/zz_lint_canary.go:1:1: func lintCanarySep2certUnused is unused (unused)"
  run bash "$SCRIPT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"planted violation not reported: lintCanaryCcmserverUnused"* ]]
}

@test "fails when the sep2cert canary is not reported" {
  # Each @test runs in its own bats subshell by design; "run" reads this
  # export in that same subshell, so the value is not actually lost.
  # shellcheck disable=SC2030,SC2031
  export STUB_GO_OUTPUT="pkg/sep2tls/ccmserver.go:1:1: func lintCanaryCcmserverUnused is unused (unused)"
  run bash "$SCRIPT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"planted violation not reported: lintCanarySep2certUnused"* ]]
}

@test "fails when the gotls fork canary is reported" {
  # Each @test runs in its own bats subshell by design; "run" reads this
  # export in that same subshell, so the value is not actually lost.
  # shellcheck disable=SC2030,SC2031
  export STUB_GO_OUTPUT="$STUB_GO_OUTPUT
pkg/sep2tls/gotls/zz_lint_canary.go:1:1: func lintCanaryGotlsUnused is unused (unused)"
  run bash "$SCRIPT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"vendored fork file was reported by golangci-lint: pkg/sep2tls/gotls/zz_lint_canary.go"* ]]
}

@test "fails when the ccm fork canary is reported" {
  # Each @test runs in its own bats subshell by design; "run" reads this
  # export in that same subshell, so the value is not actually lost.
  # shellcheck disable=SC2030,SC2031
  export STUB_GO_OUTPUT="$STUB_GO_OUTPUT
pkg/sep2tls/ccm/zz_lint_canary.go:1:1: func lintCanaryCcmUnused is unused (unused)"
  run bash "$SCRIPT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"vendored fork file was reported by golangci-lint: pkg/sep2tls/ccm/zz_lint_canary.go"* ]]
}

@test "the canary run is pinned to the golangci-lint version in ci.yml's Lint step" {
  STUB_GO_ARGV_FILE="$WORKDIR/argv.txt"
  export STUB_GO_ARGV_FILE
  run bash "$SCRIPT"
  [ "$status" -eq 0 ]
  grep -q 'golangci-lint@v2.12.2' "$STUB_GO_ARGV_FILE"
}

@test "fails closed when the ci workflow has no golangci-lint version pin" {
  sed -i '/version: v2.12.2/d' .github/workflows/ci.yml
  run bash "$SCRIPT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"no golangci-lint version pin"* ]]
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
