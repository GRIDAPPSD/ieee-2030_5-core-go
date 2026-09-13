#!/usr/bin/env bash
# Keeps the golangci-lint gate honest after merge. Two independent checks:
#   (1) canary: for every first-party package `go list ./...` reports
#       (excluding the pkg/sep2tls/gotls and pkg/sep2tls/ccm vendored
#       forks), plants a file with one finding per enabled linter
#       (errcheck, govet, ineffassign, staticcheck, unused) plus a test
#       file with one errcheck finding, and plants one unused-function and
#       one errcheck finding in each fork directory. It then runs the
#       committed .golangci.yml with the golangci-lint binary already on
#       PATH (the Lint step installs it) against a scratch copy of the
#       tracked tree, and checks: every first-party pair is reported as an
#       issue line anchored on its file and linter; no fork pair is
#       reported; golangci-lint exits exactly 1; and no level=warning or
#       level=error line appears (a diff-based issues filter such as
#       new-from-rev logs exactly such a line against a tree with no
#       .git, so this also catches that class of weakening).
#   (2) workflow: the lint job still runs golangci-lint to completion, with
#       no continue-on-error, if: false, only-new-issues, or an
#       issues-exit-code override in the action's args.
# Limits: this check does not see a rule written to spare the planted
# files specifically (for example a path-except naming the canary file),
# an exclusion narrower than a package, an in-source //nolint directive,
# a linter setting narrowed without touching a plant, a linter beyond the
# five above, a staticcheck check other than the one planted, or a
# lint-job weakening spelled differently from check (2)'s literal forms.
# It runs inside the job it guards, and main is unprotected, so a
# workflow-level change that disables the Lint job and this check
# together is not caught by anything running inside that same job; only
# branch protection or review closes that gap.
set -euo pipefail

GOLANGCI_CONFIG=".golangci.yml"
CI_WORKFLOW="${CI_WORKFLOW:-.github/workflows/ci.yml}"

FORK_PREFIXES=("pkg/sep2tls/gotls" "pkg/sep2tls/ccm")
CANARY_FILE_NAME="zz_lint_canary.go"
CANARY_TEST_FILE_NAME="zz_lint_canary_test.go"

require_file() {
  local path="$1"
  if [ ! -f "$path" ]; then
    echo "error: required file not found: $path" >&2
    exit 1
  fi
}

# is_fork_dir reports whether a scratch-relative directory is the fork
# root itself or lives under it, so a package dir of exactly
# "pkg/sep2tls/gotls" is excluded along with everything below it.
is_fork_dir() {
  local dir="$1" prefix
  for prefix in "${FORK_PREFIXES[@]}"; do
    case "$dir" in
      "$prefix" | "$prefix"/*) return 0 ;;
    esac
  done
  return 1
}

extract_lint_job_block() {
  local workflow="$1"
  awk '
    /^  lint:[[:space:]]*$/ { grab = 1; print; next }
    grab && /^  [A-Za-z0-9_-]+:[[:space:]]*$/ { grab = 0 }
    grab { print }
  ' "$workflow"
}

build_scratch_tree() {
  local scratch="$1"
  local file_list
  if ! file_list=$(git ls-files); then
    echo "error: git ls-files failed while building the canary scratch tree" >&2
    exit 1
  fi
  if [ -z "$file_list" ]; then
    echo "error: git ls-files returned no tracked files; refusing to run the canary against an empty tree" >&2
    exit 1
  fi
  local file
  while IFS= read -r file; do
    mkdir -p "$scratch/$(dirname "$file")"
    cp -p -- "$file" "$scratch/$file"
  done <<<"$file_list"
}

# discover_packages prints "<scratch-relative dir>|<package name>" for
# every package go list ./... reports in the scratch tree, one per line,
# excluding the fork directories. Fails closed on a go list error.
discover_packages() {
  local scratch="$1"
  local raw
  if ! raw=$(cd "$scratch" && go list -f '{{.Dir}}|{{.Name}}' ./... 2>&1); then
    echo "error: go list ./... failed while discovering packages in the scratch tree" >&2
    echo "$raw" >&2
    return 1
  fi
  local dir name rel
  while IFS='|' read -r dir name; do
    [ -z "$dir" ] && continue
    rel=$(realpath --relative-to="$scratch" "$dir")
    is_fork_dir "$rel" && continue
    printf '%s|%s\n' "$rel" "$name"
  done <<<"$raw"
}

# plant_first_party writes one finding for each of the five enabled
# linters into a non-test file, and one errcheck finding into a test
# file, in the given scratch-relative package directory. lintCanaryUnused
# is deliberately never referenced, so it is the only planted function
# the `unused` linter reports on; the rest are called from init() so they
# are not also flagged as unused.
plant_first_party() {
  local scratch="$1" dir="$2" name="$3"
  cat >"$scratch/$dir/$CANARY_FILE_NAME" <<EOF
package $name

import (
	"fmt"
	"os"
)

func lintCanaryUnused() {}

func lintCanaryErrcheck() {
	os.Remove("")
}

func lintCanaryGovet() {
	fmt.Printf("%d\n", "x")
}

func lintCanaryIneffassign() int {
	x := 1
	x = 2
	return x
}

func lintCanaryStaticcheck() bool {
	b := true
	return b == true
}

func init() {
	lintCanaryErrcheck()
	lintCanaryGovet()
	_ = lintCanaryIneffassign()
	_ = lintCanaryStaticcheck()
}
EOF
  cat >"$scratch/$dir/$CANARY_TEST_FILE_NAME" <<EOF
package $name

import "os"

func lintCanaryTestErrcheck() {
	os.Remove("")
}

func init() {
	lintCanaryTestErrcheck()
}
EOF
}

# plant_fork_dir writes one unreferenced (unused) function and one
# referenced errcheck finding into a fork directory, so a weakened fork
# exclusion shows up the same way an accidental one would: an issue line
# naming a file under that directory.
plant_fork_dir() {
  local scratch="$1" dir="$2" name="$3" suffix="$4"
  if [ ! -d "$scratch/$dir" ]; then
    echo "error: expected fork directory not found in scratch tree: $dir" >&2
    exit 1
  fi
  cat >"$scratch/$dir/$CANARY_FILE_NAME" <<EOF
package $name

import "os"

func lintCanary${suffix}Unused() {}

func lintCanary${suffix}Errcheck() {
	os.Remove("")
}

func init() {
	lintCanary${suffix}Errcheck()
}
EOF
}

run_canary_lint() {
  local scratch="$1"
  # A cache scoped to this scratch tree: golangci-lint's build cache keys
  # on package content, and a cache shared across scratch trees with
  # byte-identical files but different roots has been observed to replay
  # a stale run's file paths instead of relinting. Fresh cache, fresh
  # scratch tree, every invocation.
  local cache="$scratch/.golangci-lint-cache"
  mkdir -p "$cache"
  # golangci-lint is expected to exit 1 here (real findings are planted);
  # check the captured status explicitly rather than trusting errexit,
  # which would otherwise abort the script on the expected case.
  set +e
  CANARY_LINT_OUTPUT=$(cd "$scratch" && GOLANGCI_LINT_CACHE="$cache" golangci-lint run ./... 2>&1)
  CANARY_LINT_STATUS=$?
  set -e
}

# pair_reported checks for an issue line anchored at the start as
# "<path>:<line>:<col>: ... (<linter>)", so a planted marker appearing
# only inside a message body (never as the leading path) does not count.
pair_reported() {
  local path="$1" linter="$2"
  printf '%s\n' "$CANARY_LINT_OUTPUT" | grep -qE "^${path}:[0-9]+:[0-9]+: .*\(${linter}\)\$"
}

# file_reported checks whether any issue line is anchored on the given
# file, regardless of linter.
file_reported() {
  local path="$1"
  printf '%s\n' "$CANARY_LINT_OUTPUT" | grep -qE "^${path}:[0-9]+:[0-9]+: "
}

check_canary() {
  local config="$1" scratch="$2"
  require_file "$config"

  if ! command -v golangci-lint >/dev/null 2>&1; then
    echo "error: golangci-lint is not on PATH; this step must run after the Lint step so the action-installed binary is available" >&2
    return 1
  fi
  echo "golangci-lint on PATH: $(golangci-lint version)"

  build_scratch_tree "$scratch"

  local discovered
  if ! discovered=$(discover_packages "$scratch"); then
    return 1
  fi
  local pkg_count=0
  local dir name
  local -a expected=()
  local -a first_party_dirs=()
  while IFS='|' read -r dir name; do
    [ -z "$dir" ] && continue
    pkg_count=$((pkg_count + 1))
    first_party_dirs+=("$dir")
    plant_first_party "$scratch" "$dir" "$name"
    expected+=(
      "$dir/$CANARY_FILE_NAME|unused"
      "$dir/$CANARY_FILE_NAME|errcheck"
      "$dir/$CANARY_FILE_NAME|govet"
      "$dir/$CANARY_FILE_NAME|ineffassign"
      "$dir/$CANARY_FILE_NAME|staticcheck"
      "$dir/$CANARY_TEST_FILE_NAME|errcheck"
    )
  done <<<"$discovered"

  if [ "$pkg_count" -eq 0 ]; then
    echo "error: go list ./... in the scratch tree reported 0 first-party packages outside ${FORK_PREFIXES[*]}; refusing to run a canary with nothing to plant" >&2
    return 1
  fi
  echo "discovered $pkg_count first-party package(s):"
  printf '  %s\n' "${first_party_dirs[@]}"

  plant_fork_dir "$scratch" "${FORK_PREFIXES[0]}" "gotls" "Gotls"
  plant_fork_dir "$scratch" "${FORK_PREFIXES[1]}" "ccm" "Ccm"
  require_file "$scratch/${FORK_PREFIXES[0]}/$CANARY_FILE_NAME"
  require_file "$scratch/${FORK_PREFIXES[1]}/$CANARY_FILE_NAME"

  run_canary_lint "$scratch"

  local fail=0

  if [ "$CANARY_LINT_STATUS" -ne 1 ]; then
    echo "FAIL: golangci-lint exited $CANARY_LINT_STATUS against the canary tree; expected exactly 1 (issues found, no tool error and no issues-exit-code override)" >&2
    fail=1
  fi

  local warn_lines
  warn_lines=$(printf '%s\n' "$CANARY_LINT_OUTPUT" | grep -E 'level=(warning|error)' || true)
  if [ -n "$warn_lines" ]; then
    echo "FAIL: golangci-lint printed a level=warning or level=error line:" >&2
    echo "$warn_lines" >&2
    fail=1
  fi

  local pair path linter
  for pair in "${expected[@]}"; do
    path="${pair%|*}"
    linter="${pair#*|}"
    if ! pair_reported "$path" "$linter"; then
      echo "FAIL: planted violation not reported: $linter in $path" >&2
      fail=1
    fi
  done

  local forkfile
  for forkfile in "${FORK_PREFIXES[0]}/$CANARY_FILE_NAME" "${FORK_PREFIXES[1]}/$CANARY_FILE_NAME"; do
    if file_reported "$forkfile"; then
      echo "FAIL: vendored fork file was reported by golangci-lint: $forkfile; the fork exclusion is not scoped as expected" >&2
      fail=1
    fi
  done

  if [ "$fail" -ne 0 ]; then
    # Surfaces the underlying output verbatim (a git or go error names
    # itself) alongside the curated FAIL lines above.
    echo "canary golangci-lint invocation: exit $CANARY_LINT_STATUS" >&2
    echo "$CANARY_LINT_OUTPUT" >&2
  fi

  return "$fail"
}

check_lint_job_not_weakened() {
  local workflow="$1"
  local block
  block=$(extract_lint_job_block "$workflow")
  if [ -z "$block" ]; then
    echo "error: no 'lint:' job found in $workflow" >&2
    return 1
  fi

  local fail=0

  if grep -qiE "continue-on-error:[[:space:]]*[\"']?true[\"']?([[:space:]]|\$)" <<<"$block" \
    || grep -qiE 'continue-on-error:[[:space:]]*\$\{\{[[:space:]]*true[[:space:]]*\}\}' <<<"$block"; then
    echo "FAIL: the lint job in $workflow sets continue-on-error to true" >&2
    fail=1
  fi

  if grep -qE '^[[:space:]]*if:[[:space:]]*false[[:space:]]*$' <<<"$block"; then
    echo "FAIL: the lint job in $workflow has a step or job condition of if: false" >&2
    fail=1
  fi

  if ! grep -qE 'uses:[[:space:]]*golangci/golangci-lint-action@' <<<"$block"; then
    echo "FAIL: no golangci-lint-action step found in the lint job in $workflow" >&2
    fail=1
  fi

  if grep -qiE 'only-new-issues:[[:space:]]*true' <<<"$block"; then
    echo "FAIL: the lint job in $workflow sets only-new-issues: true, which hides pre-existing findings" >&2
    fail=1
  fi

  if grep -qE -- '(^|[[:space:]])(--)?issues-exit-code[=[:space:]]+0([[:space:]]|$)' <<<"$block"; then
    echo "FAIL: the lint job in $workflow overrides issues-exit-code to 0 via step args" >&2
    fail=1
  fi

  return "$fail"
}

main() {
  require_file "$GOLANGCI_CONFIG"
  require_file "$CI_WORKFLOW"

  local fail=0
  # Not "local": the EXIT trap below runs after main returns, once this
  # function's locals are gone, so the cleanup path needs a global.
  SCRATCH_DIR="$(mktemp -d)"
  trap 'rm -rf "$SCRATCH_DIR"' EXIT

  check_canary "$GOLANGCI_CONFIG" "$SCRATCH_DIR" || fail=1
  check_lint_job_not_weakened "$CI_WORKFLOW" || fail=1

  if [ "$fail" -ne 0 ]; then
    exit 1
  fi

  echo "lint gate check passed: golangci-lint reported every planted first-party finding and no fork finding, exited exactly 1 with no level=warning or level=error line, and the lint job has no continue-on-error, if: false, only-new-issues, or issues-exit-code override."
}

main "$@"
