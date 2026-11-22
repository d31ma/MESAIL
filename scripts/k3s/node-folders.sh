#!/usr/bin/env bash
# Usage, as root in the dev host: mesail-devhost exec sudo bash -s < scripts/k3s/node-folders.sh
set -Eeuo pipefail

node_folders_names() {
  printf '%s\n' directory channels messaging attachments notify search mail-in mail-out devicebox keycloak blobs
}

node_folders_data_root() {
  echo "${MESAIL_FOLDERS_ROOT:-/mnt/mesail-data}"
}

node_folders_owner() {
  echo "${MESAIL_FOLDERS_OWNER:-10001:10001}"
}

node_folders_require_data_disk() {
  if ! mountpoint -q "$(node_folders_data_root)"; then
    echo "node-folders: $(node_folders_data_root) is not a mount point; refusing, a folder would land on the VM's OS disk" >&2
    return 1
  fi
}

node_folders_found_owner() {
  # shellcheck disable=SC2012
  ls -nd "$1" | awk '{print $3 ":" $4}'
}

node_folders_make() {
  local path="$1" owner
  owner="$(node_folders_owner)"
  install -d -m 0700 -o "${owner%%:*}" -g "${owner##*:}" "$path"
  echo "created $path"
}

node_folders_check_existing() {
  local path="$1" found
  found="$(node_folders_found_owner "$path")"
  if [ ! -d "$path" ] || [ "$found" != "$(node_folders_owner)" ]; then
    echo "node-folders: $path exists with owner $found (expected a folder owned by $(node_folders_owner)); refusing, nothing changed" >&2
    return 1
  fi
  echo "ok      $path"
}

node_folders_ensure() {
  local path="$1"
  if [ -e "$path" ]; then
    node_folders_check_existing "$path"
    return
  fi
  node_folders_make "$path"
}

node_folders_main() {
  local apps name
  node_folders_require_data_disk
  apps="$(node_folders_data_root)/apps"
  install -d -m 0755 "$apps"
  for name in $(node_folders_names); do
    node_folders_ensure "$apps/$name.vol"
  done
  echo "node-folders: done"
}

node_folders_main "$@"
