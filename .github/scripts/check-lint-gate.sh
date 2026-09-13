#!/usr/bin/env bash
# Keeps the golangci-lint gate honest after merge. Fails when:
#   (a) a linters.exclusions.paths entry in .golangci.yml matches a
#       tracked .go file outside the two vendored fork directories;
#   (b) the Lint job or any of its steps sets continue-on-error: true;
#   (c) .golangci.yml sets issues-exit-code to 0.
# Any of the three would let the Lint job report green while a real
# first-party finding goes unreported.
set -euo pipefail

GOLANGCI_CONFIG="${GOLANGCI_CONFIG:-.golangci.yml}"
CI_WORKFLOW="${CI_WORKFLOW:-.github/workflows/ci.yml}"
ALLOWED_PREFIXES=("pkg/sep2tls/gotls/" "pkg/sep2tls/ccm/")

require_file() {
  local path="$1"
  if [ ! -f "$path" ]; then
    echo "error: required file not found: $path" >&2
    exit 1
  fi
}

extract_exclusion_patterns() {
  local config="$1"
  awk '
    /^[[:space:]]*paths:[[:space:]]*$/ { in_paths=1; next }
    in_paths && /^[[:space:]]*-[[:space:]]*[^[:space:]#]/ {
      line = $0
      sub(/^[[:space:]]*-[[:space:]]*/, "", line)
      sub(/[[:space:]]*#.*$/, "", line)
      gsub(/^"|"$/, "", line)
      gsub(/^'"'"'|'"'"'$/, "", line)
      print line
      next
    }
    in_paths && /^[[:space:]]*#/ { next }
    in_paths && /^[[:space:]]*[^-[:space:]]/ { in_paths = 0 }
  ' "$config"
}

extract_lint_job_block() {
  local workflow="$1"
  awk '
    /^  lint:[[:space:]]*$/ { grab = 1; print; next }
    grab && /^  [A-Za-z0-9_-]+:[[:space:]]*$/ { grab = 0 }
    grab { print }
  ' "$workflow"
}

check_exclusions_scoped(){
  local config="$1"
  local -n allowed_ref="$2"
  local fail=0
  local -a patterns=()
  local -a tracked_go_files=()

  mapfile -t patterns < <(extract_exclusion_patterns "$config")
  if [ "${#patterns[@]}" -eq 0 ]; then
    echo "error: no linters.exclusions.paths entries found in $config; cannot verify gate scope" >&2
    return 1
  fi

  mapfile -t tracked_go_files < <(git ls-files -- '*.go')

  local pattern file prefix allowed
  for pattern in "${patterns[@]}"; do
    for file in "${tracked_go_files[@]}"; do
      if [[ "$file" =~ $pattern ]]; then
        allowed=0
        for prefix in "${allowed_ref[@]}"; do
          case "$file" in
            "$prefix"*) allowed=1 ;;
          esac
        done
        if [ "$allowed" -eq 0 ]; then
          echo "FAIL: exclusion pattern '$pattern' in $config matches tracked file outside the fork directories: $file" >&2
          fail=1
        fi
      fi
    done
  done

  return "$fail"
}

check_no_continue_on_error() {
  local workflow="$1"
  local block
  block=$(extract_lint_job_block "$workflow")
  if [ -z "$block" ]; then
    echo "error: no 'lint:' job found in $workflow" >&2
    return 1
  fi
  if grep -qE 'continue-on-error:[[:space:]]*true' <<<"$block"; then
    echo "FAIL: the lint job in $workflow sets continue-on-error: true" >&2
    return 1
  fi
  return 0
}

check_no_silent_issues_exit_code() {
  local config="$1"
  if grep -qE '^[[:space:]]*issues-exit-code:[[:space:]]*0[[:space:]]*$' "$config"; then
    echo "FAIL: $config sets issues-exit-code: 0, which reports findings without failing the run" >&2
    return 1
  fi
  return 0
}

main() {
  require_file "$GOLANGCI_CONFIG"
  require_file "$CI_WORKFLOW"

  local fail=0

  check_exclusions_scoped "$GOLANGCI_CONFIG" ALLOWED_PREFIXES || fail=1
  check_no_continue_on_error "$CI_WORKFLOW" || fail=1
  check_no_silent_issues_exit_code "$GOLANGCI_CONFIG" || fail=1

  if [ "$fail" -ne 0 ]; then
    exit 1
  fi

  echo "lint gate check passed: exclusions scoped to ${ALLOWED_PREFIXES[*]}, continue-on-error absent from the lint job, issues-exit-code not silenced."
}

main "$@"
