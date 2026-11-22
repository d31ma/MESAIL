#!/usr/bin/env bash
# shellcheck shell=bash
set -uo pipefail

node_folders_test_fail() {
  echo "FAIL $1: $2" >&2
  exit 1
}

node_folders_test_expect_equal() {
  local name="$1" expected="$2" actual="$3"
  if [ "$expected" != "$actual" ]; then node_folders_test_fail "$name" "expected '$expected', got '$actual'"; fi
  echo "ok   $name"
}

node_folders_test_script() {
  echo "$(cd "$(dirname "$0")" && pwd)/node-folders.sh"
}

node_folders_test_fake_bin() {
  local folder="$1" answer="$2"
  mkdir -p "$folder"
  printf '#!/bin/sh\nexit %s\n' "$answer" > "$folder/mountpoint"
  chmod +x "$folder/mountpoint"
}

node_folders_test_run() {
  local root="$1" bin="$2" owner="$3"
  MESAIL_FOLDERS_ROOT="$root" MESAIL_FOLDERS_OWNER="$owner" PATH="$bin:$PATH" bash "$(node_folders_test_script)" > "$root.out" 2>&1
  echo "$?"
}

node_folders_test_count() {
  find "$1/apps" -mindepth 1 -maxdepth 1 -type d -name '*.vol' 2> /dev/null | wc -l | tr -d ' '
}

node_folders_test_mode() {
  # shellcheck disable=SC2012
  ls -ld "$1" | cut -c1-10
}

node_folders_test_has() {
  if test "$1" "$2"; then
    echo yes
    return
  fi
  echo no
}

node_folders_test_main() {
  local work me other
  work="$(mktemp -d)"
  # shellcheck disable=SC2064
  trap "rm -rf '$work'" EXIT
  me="$(id -u):$(id -g)"
  other="4242:4242"
  node_folders_test_fake_bin "$work/no-mount" 1
  node_folders_test_fake_bin "$work/mount" 0
  mkdir -p "$work/disk"

  node_folders_test_expect_equal "refuses when the data disk is not mounted" 1 "$(node_folders_test_run "$work/disk" "$work/no-mount" "$me")"
  node_folders_test_expect_equal "makes nothing when the data disk is not mounted" 0 "$(node_folders_test_count "$work/disk")"

  node_folders_test_expect_equal "first run succeeds" 0 "$(node_folders_test_run "$work/disk" "$work/mount" "$me")"
  node_folders_test_expect_equal "makes 11 app folders" 11 "$(node_folders_test_count "$work/disk")"
  node_folders_test_expect_equal "folders have mode 0700" "drwx------" "$(node_folders_test_mode "$work/disk/apps/directory.vol")"
  node_folders_test_expect_equal "keycloak.vol exists" yes "$(node_folders_test_has -d "$work/disk/apps/keycloak.vol")"
  node_folders_test_expect_equal "blobs.vol exists" yes "$(node_folders_test_has -d "$work/disk/apps/blobs.vol")"

  touch "$work/disk/apps/messaging.vol/data-file"
  node_folders_test_expect_equal "second run succeeds" 0 "$(node_folders_test_run "$work/disk" "$work/mount" "$me")"
  node_folders_test_expect_equal "second run deletes nothing" yes "$(node_folders_test_has -f "$work/disk/apps/messaging.vol/data-file")"

  node_folders_test_expect_equal "refuses folders with another owner" 1 "$(node_folders_test_run "$work/disk" "$work/mount" "$other")"
  node_folders_test_expect_equal "refusal keeps the data" yes "$(node_folders_test_has -f "$work/disk/apps/messaging.vol/data-file")"

  rm -rf "$work/disk/apps/search.vol"
  touch "$work/disk/apps/search.vol"
  node_folders_test_expect_equal "refuses a file where a folder belongs" 1 "$(node_folders_test_run "$work/disk" "$work/mount" "$me")"
  node_folders_test_expect_equal "the file stays a file" yes "$(node_folders_test_has -f "$work/disk/apps/search.vol")"
}

node_folders_test_main "$@"
