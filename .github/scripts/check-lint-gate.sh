#!/usr/bin/env bash
# Keeps the golangci-lint gate honest after merge. Two independent checks:
#   (1) canary: plants an unused function beside each vendored fork
#       directory and one elsewhere in the module, then runs golangci-lint
#       itself, pinned to the version ci.yml's Lint step uses, against a
#       scratch copy of the tree. Any exclusion pattern or issues-exit-code
#       setting that would hide or silence a first-party finding shows up
#       as a missing marker or a zero exit, because it is golangci-lint's
#       own behavior being read, not a parse of .golangci.yml.
#   (2) workflow: the lint job still runs golangci-lint to completion, with
#       no continue-on-error, if: false, only-new-issues, or an
#       issues-exit-code override in the action's args.
# Limit: (2) is a scan of ci.yml in the same PR that could edit it, and
# main is unprotected, so a workflow-level change that disables both the
# lint job and this check together is not caught by anything running
# inside that same job. Only branch protection or review closes that gap.
set -euo pipefail

GOLANGCI_CONFIG="${GOLANGCI_CONFIG:-.golangci.yml}"
CI_WORKFLOW="${CI_WORKFLOW:-.github/workflows/ci.yml}"

# Canary sites. The first-party pair sits beside the fork directories,
# which is exactly the substring collision an unanchored or merged
# exclusion pattern has hidden before (see .golangci.yml's own comment).
CANARY_APPEND_FILE="pkg/sep2tls/ccmserver.go"
CANARY_APPEND_MARKER="lintCanaryCcmserverUnused"
CANARY_NEW_FILE="pkg/sep2cert/zz_lint_canary.go"
CANARY_NEW_PACKAGE="sep2cert"
CANARY_NEW_MARKER="lintCanarySep2certUnused"
FORK_GOTLS_FILE="pkg/sep2tls/gotls/zz_lint_canary.go"
FORK_GOTLS_MARKER="lintCanaryGotlsUnused"
FORK_CCM_FILE="pkg/sep2tls/ccm/zz_lint_canary.go"
FORK_CCM_MARKER="lintCanaryCcmUnused"
LINT_SCOPE=("./pkg/sep2tls/..." "./pkg/sep2cert/...")

require_file() {
  local path="$1"
  if [ ! -f "$path" ]; then
    echo "error: required file not found: $path" >&2
    exit 1
  fi
}

extract_lint_job_block() {
  local workflow="$1"
  awk '
    /^  lint:[[:space:]]*$/ { grab = 1; print; next }
    grab && /^  [A-Za-z0-9_-]+:[[:space:]]*$/ { grab = 0 }
    grab { print }
  ' "$workflow"
}

golangci_lint_version() {
  local workflow="$1"
  local block version
  block=$(extract_lint_job_block "$workflow")
  version=$(grep -oE 'version:[[:space:]]*v[0-9]+\.[0-9]+\.[0-9]+' <<<"$block" | head -n1 | awk '{print $2}')
  if [ -z "$version" ]; then
    echo "error: no golangci-lint version pin (version: vX.Y.Z) found in the lint job in $workflow" >&2
    # A command substitution's "exit" only ends its own subshell, not the
    # script, so this precondition is signalled with "return" and checked
    # explicitly by the caller instead.
    return 1
  fi
  printf '%s' "$version"
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

plant_canaries() {
  local scratch="$1"

  require_file "$scratch/$CANARY_APPEND_FILE"
  printf '\nfunc %s() {}\n' "$CANARY_APPEND_MARKER" >>"$scratch/$CANARY_APPEND_FILE"

  if [ ! -d "$scratch/$(dirname "$CANARY_NEW_FILE")" ]; then
    echo "error: expected package directory not found in scratch tree: $(dirname "$CANARY_NEW_FILE")" >&2
    exit 1
  fi
  printf 'package %s\n\nfunc %s() {}\n' "$CANARY_NEW_PACKAGE" "$CANARY_NEW_MARKER" >"$scratch/$CANARY_NEW_FILE"

  if [ ! -d "$scratch/$(dirname "$FORK_GOTLS_FILE")" ]; then
    echo "error: expected fork directory not found in scratch tree: $(dirname "$FORK_GOTLS_FILE")" >&2
    exit 1
  fi
  printf 'package gotls\n\nfunc %s() {}\n' "$FORK_GOTLS_MARKER" >"$scratch/$FORK_GOTLS_FILE"

  if [ ! -d "$scratch/$(dirname "$FORK_CCM_FILE")" ]; then
    echo "error: expected fork directory not found in scratch tree: $(dirname "$FORK_CCM_FILE")" >&2
    exit 1
  fi
  printf 'package ccm\n\nfunc %s() {}\n' "$FORK_CCM_MARKER" >"$scratch/$FORK_CCM_FILE"
}

run_canary_lint() {
  local scratch="$1" version="$2"
  # A cache scoped to this scratch tree: golangci-lint's build cache keys
  # on package content, and a cache shared across scratch trees with
  # byte-identical files but different roots has been observed to replay
  # a stale run's file paths instead of relinting. Fresh cache, fresh scratch
  # tree, every invocation.
  local cache="$scratch/.golangci-lint-cache"
  mkdir -p "$cache"
  # golangci-lint is expected to exit non-zero here (real findings are
  # planted); check the captured status explicitly rather than trusting
  # errexit, which would otherwise abort the script on the expected case.
  set +e
  CANARY_LINT_OUTPUT=$(cd "$scratch" && GOLANGCI_LINT_CACHE="$cache" go run "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@${version}" run "${LINT_SCOPE[@]}" 2>&1)
  CANARY_LINT_STATUS=$?
  set -e
}

check_canary() {
  local config="$1" workflow="$2" scratch="$3"
  require_file "$config"
  local version
  if ! version="$(golangci_lint_version "$workflow")"; then
    return 1
  fi

  build_scratch_tree "$scratch"
  plant_canaries "$scratch"
  run_canary_lint "$scratch" "$version"

  local fail=0

  if [ "$CANARY_LINT_STATUS" -eq 0 ]; then
    echo "FAIL: golangci-lint exited 0 against the canary tree with planted first-party violations present; issues-exit-code may be silenced" >&2
    fail=1
  fi
  case "$CANARY_LINT_OUTPUT" in
    *"$CANARY_APPEND_MARKER"*) ;;
    *)
      echo "FAIL: planted violation not reported: $CANARY_APPEND_MARKER in $CANARY_APPEND_FILE (an exclusion pattern may have widened to hide it)" >&2
      fail=1
      ;;
  esac
  case "$CANARY_LINT_OUTPUT" in
    *"$CANARY_NEW_MARKER"*) ;;
    *)
      echo "FAIL: planted violation not reported: $CANARY_NEW_MARKER in $CANARY_NEW_FILE" >&2
      fail=1
      ;;
  esac
  case "$CANARY_LINT_OUTPUT" in
    *"$FORK_GOTLS_MARKER"*)
      echo "FAIL: vendored fork file was reported by golangci-lint: $FORK_GOTLS_FILE; the fork exclusion is not scoped as expected" >&2
      fail=1
      ;;
  esac
  case "$CANARY_LINT_OUTPUT" in
    *"$FORK_CCM_MARKER"*)
      echo "FAIL: vendored fork file was reported by golangci-lint: $FORK_CCM_FILE; the fork exclusion is not scoped as expected" >&2
      fail=1
      ;;
  esac

  if [ "$fail" -ne 0 ]; then
    # Surfaces the underlying failure verbatim (a git or go error names
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

  check_canary "$GOLANGCI_CONFIG" "$CI_WORKFLOW" "$SCRATCH_DIR" || fail=1
  check_lint_job_not_weakened "$CI_WORKFLOW" || fail=1

  if [ "$fail" -ne 0 ]; then
    exit 1
  fi

  echo "lint gate check passed: golangci-lint reported the planted canary violations without hiding them or exiting 0, and the lint job has no continue-on-error, if: false, only-new-issues, or issues-exit-code override."
}

main "$@"
