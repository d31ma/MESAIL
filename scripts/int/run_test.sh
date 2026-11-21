#!/usr/bin/env bash
set -uo pipefail

int_test_script() {
  echo "$(cd "$(dirname "$0")" && pwd)/run.sh"
}

int_test_fail() {
  echo "FAIL $1: $2" >&2
  exit 1
}

int_test_expect_status() {
  local name="$1" expected="$2" actual="$3"
  if [ "$expected" != "$actual" ]; then int_test_fail "$name" "expected status $expected, got $actual"; fi
  echo "ok   $name"
}

int_test_expect_contains() {
  local name="$1" text="$2" file="$3"
  if ! grep -q -F -- "$text" "$file"; then int_test_fail "$name" "'$text' not in $(cat "$file")"; fi
  echo "ok   $name"
}

int_test_expect_missing() {
  local name="$1" path="$2"
  if [ -e "$path" ]; then int_test_fail "$name" "$path exists"; fi
  echo "ok   $name"
}

int_test_only_summary() {
  find "$1" -name summary.md | head -1
}

int_test_rejects() {
  local work="$1" name="$2" text="$3" status
  shift 3
  MESAIL_INT_LOGS="$work/logs" "$(int_test_script)" "$@" > "$work/out" 2>&1
  status=$?
  int_test_expect_status "$name exits 1" 1 "$status"
  int_test_expect_contains "$name says why" "$text" "$work/out"
}

int_test_arguments() {
  local work="$1"
  int_test_rejects "$work" "an unknown step" "--steps takes native, a11y, menus, floci" --steps native,deploy
  int_test_rejects "$work" "a file without its hash" "--ty and --ty-sha256 go together" --ty "$work/ty"
  int_test_rejects "$work" "a short hash" "--fylo-sha256 must be 64 lowercase hex characters" --fylo "$work/ty" --fylo-sha256 abc
  int_test_rejects "$work" "a short DuVay commit" "--duvay must be a full 40-character commit SHA" --duvay e08ae67
  int_test_rejects "$work" "a bad app list" "--native-apps takes a comma list of app names" --native-apps 'directory;rm'
  int_test_rejects "$work" "an unknown option" "unknown option --fast" --fast
}

int_test_checksum_mismatch() {
  local work="$1" status summary wrong
  wrong="$(printf '0%.0s' $(seq 64))"
  printf 'search\t^TestAcSr\nsome future flow\t\n' > "$work/flows.tsv"
  MESAIL_INT_FLOWS="$work/flows.tsv" MESAIL_INT_LOGS="$work/mismatch" MESAIL_INT_WORK="$work/trees" "$(int_test_script)" \
    --ty-linux "$work/ty" --ty-linux-sha256 "$wrong" --tachyon-commit abc123 --steps floci > "$work/mismatch.out" 2>&1
  status=$?
  summary="$(int_test_only_summary "$work/mismatch")"
  int_test_expect_status "a checksum mismatch fails the run" 1 "$status"
  int_test_expect_contains "the summary says RED" ": RED" "$summary"
  int_test_expect_contains "the builds step failed" "| builds | FAIL rc=1 |" "$summary"
  int_test_expect_contains "the failure names the mismatch" "checksum mismatch for $work/ty" "$summary"
  int_test_expect_contains "the summary names the TACHYON commit" "- TACHYON: commit abc123" "$summary"
  int_test_expect_contains "an uncovered flow is listed as not covered" "| some future flow | not covered (no blackbox test yet) |" "$summary"
  int_test_expect_contains "a covered flow that did not run is not a pass" "| search | not run |" "$summary"
  int_test_expect_missing "no worktree is made" "$work/trees"
  int_test_expect_missing "no tool copy is left" "$(dirname "$summary")/tools"
}

int_test_missing_clone() {
  local work="$1" status summary
  MESAIL_INT_LOGS="$work/noclone" MESAIL_INT_WORK="$work/trees" MESAIL_INT_CLONE="$work/no-clone" "$(int_test_script)" \
    --ty-linux "$work/ty" --ty-linux-sha256 "$(shasum -a 256 "$work/ty" | awk '{ print $1 }')" --steps floci > "$work/noclone.out" 2>&1
  status=$?
  summary="$(int_test_only_summary "$work/noclone")"
  int_test_expect_status "a missing clone fails the run" 1 "$status"
  int_test_expect_contains "a matching hash passes the builds step" "| builds | pass |" "$summary"
  int_test_expect_contains "the worktree step failed" "| worktree | FAIL" "$summary"
}

main() {
  local work
  work="$(mktemp -d "${TMPDIR:-/tmp}/mesail-int-test.XXXXXX")"
  INT_TEST_WORK="$work"
  trap 'rm -rf "$INT_TEST_WORK"' EXIT
  printf 'not a real ty\n' > "$work/ty"
  int_test_arguments "$work"
  int_test_checksum_mismatch "$work"
  int_test_missing_clone "$work"
  echo "int harness: all checks passed"
}

main "$@"
