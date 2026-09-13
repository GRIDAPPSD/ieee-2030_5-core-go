#!/usr/bin/env bats
# Smoke and failure-path coverage for check-lint-gate.sh: a happy-path
# run, one test per documented gate defect (a, b, c), and one test per
# fail-closed precondition.

setup() {
  SCRIPT="$BATS_TEST_DIRNAME/check-lint-gate.sh"
  WORKDIR="$(mktemp -d)"
  cd "$WORKDIR" || exit 1

  git init -q
  git config user.email test@example.invalid
  git config user.name test

  mkdir -p pkg/sep2tls/gotls pkg/sep2tls/ccm .github/workflows

  echo 'package gotls' >pkg/sep2tls/gotls/foo.go
  echo 'package ccm' >pkg/sep2tls/ccm/bar.go
  echo 'package sep2tls' >pkg/sep2tls/ccmserver.go

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
}

teardown() {
  cd /
  rm -rf "$WORKDIR"
}

@test "passes when exclusions are anchored and the lint job has no continue-on-error or silent exit code" {
  run bash "$SCRIPT"
  [ "$status" -eq 0 ]
  [[ "$output" == *"lint gate check passed"* ]]
}

@test "fails when an exclusions.paths entry matches a first-party file outside the fork directories" {
  sed -i 's#- \^pkg/sep2tls/ccm/#- pkg/sep2tls/ccm#' .golangci.yml
  run bash "$SCRIPT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"matches tracked file outside the fork directories: pkg/sep2tls/ccmserver.go"* ]]
}

@test "fails when the lint job sets continue-on-error: true" {
  sed -i '/uses: golangci\/golangci-lint-action@v9/a\        continue-on-error: true' .github/workflows/ci.yml
  run bash "$SCRIPT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"continue-on-error: true"* ]]
}

@test "fails when .golangci.yml sets issues-exit-code to 0" {
  sed -i '/build-tags: \[\]/a\  issues-exit-code: 0' .golangci.yml
  run bash "$SCRIPT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"issues-exit-code: 0"* ]]
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

@test "fails closed when .golangci.yml has no exclusions.paths entries" {
  cat >.golangci.yml <<'YAML'
version: "2"
run:
  build-tags: []
YAML
  run bash "$SCRIPT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"no linters.exclusions.paths entries found"* ]]
}
