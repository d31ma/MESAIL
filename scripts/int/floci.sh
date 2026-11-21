#!/usr/bin/env bash
set -euo pipefail
# shellcheck source-path=SCRIPTDIR/../floci
. "$(cd "$(dirname "$0")/../floci" && pwd)/common.sh"

usage() {
  cat << 'TEXT'
Usage: scripts/int/floci.sh

The Floci part of an integration run (scripts/int/run.sh) on the running Mesail dev host:
build the images (MESAIL_IMAGE_TAG, MESAIL_INT_TOOLS), up.sh, deploy.sh, blobs.sh, the seed
and the blackbox tests (test.sh; MESAIL_TEST_RUN limits them), then save the container logs
and `docker compose stop` this stack. It never runs down.sh, removes no container, network or
volume, and never starts, stops or recreates the dev host or its Floci (the dev host rules).
Run it inside the mesail-docker lock; scripts/int/run.sh does.
TEXT
  exit 2
}

floci_folder() {
  cd "$(dirname "$0")/../floci" && pwd
}

devhost() {
  "$HOME/.local/bin/mesail-devhost" "$@"
}

use_running_devhost() {
  devhost status 2>&1 | grep -q '"status": "running_verified"' || { echo "the Mesail dev host is not running; ask mesail-devhost"; return 1; }
  eval "$(devhost env)"
  export MESAIL_VM=mesail MESAIL_FLOCI_PORT=14566 MESAIL_FLOCI_URL=http://127.0.0.1:14566
}

stack_profile() {
  local profile
  profile="$(cat "$(mesail_log_dir)/compose-profile")"
  if [ -s "$(mesail_log_dir)/blobs-volume" ]; then profile="$profile --file $(mesail_root)/deploy/floci/compose.blobs.yaml"; fi
  echo "$profile"
}

save_logs() {
  local container
  for container in $(docker ps --all --filter network=mesail_floci_backend --format '{{.Names}}'); do
    docker logs --timestamps "$container" > "$(mesail_log_dir)/container-$container.log" 2>&1 || true
  done
  echo "saved the logs of the function containers"
}

stop_stack() {
  if [ ! -f "$(mesail_log_dir)/compose-profile" ]; then echo "the stack was not started"; return 0; fi
  MESAIL_BLOBS_VOLUME="$(cat "$(mesail_log_dir)/blobs-volume" 2> /dev/null || true)"
  export MESAIL_BLOBS_VOLUME
  # shellcheck disable=SC2046
  docker compose --project-directory "$(mesail_root)" $(stack_profile) stop
}

left_behind() {
  echo "containers:"
  docker ps --all --format '{{.Names}} {{.Status}}' --filter network=mesail_floci_backend
  echo "volumes:"
  docker volume ls --quiet --filter name=floci-aws-efs
}

finish() {
  local code=$?
  if [ -n "${MESAIL_WRITER_GUARD:-}" ]; then kill "$MESAIL_WRITER_GUARD" 2> /dev/null || true; fi
  mesail_step save-logs save_logs || true
  mesail_step compose-stop stop_stack || code=1
  mesail_step left-behind left_behind || true
  if [ -n "${MESAIL_SECRETS_FILE:-}" ]; then rm -f "$MESAIL_SECRETS_FILE"; fi
  echo "=== int floci rc=$code (logs in $(mesail_log_dir))"
  exit "$code"
}

start_writer_guard() {
  "$(floci_folder)/writers.sh" watch 5 >> "$(mesail_log_dir)/writers.log" 2>&1 &
  MESAIL_WRITER_GUARD=$!
}

main() {
  case "${1:-}" in
    -h | --help) usage ;;
  esac
  use_running_devhost
  mesail_load_settings
  MESAIL_SECRETS_FILE="$(mesail_generate_secrets)"
  export MESAIL_SECRETS_FILE
  trap finish EXIT
  "$(floci_folder)/build.sh"
  "$(floci_folder)/up.sh"
  "$(floci_folder)/deploy.sh"
  "$(floci_folder)/blobs.sh"
  start_writer_guard
  "$(floci_folder)/test.sh"
}

main "$@"
