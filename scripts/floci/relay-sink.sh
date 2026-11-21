#!/usr/bin/env bash
set -euo pipefail
# shellcheck source-path=SCRIPTDIR
. "$(dirname "$0")/common.sh"

compose() {
  docker compose --project-directory "$(mesail_root)" "$@"
}

used_profile() {
  cat "$(mesail_log_dir)/compose-profile"
}

records_dir() {
  echo "$(mesail_log_dir)/dkim-records"
}

record_object() {
  jq -n --arg name "$(basename "$1")" --rawfile record "$1" '{($name): $record}'
}

dkim_records() {
  local file
  for file in "$(records_dir)"/*; do
    if [ -f "$file" ]; then record_object "$file"; fi
  done | jq -s -c 'add // {}'
}

listing_url() {
  echo "http://127.0.0.1:$MESAIL_RELAY_SINK_PORT/messages"
}

start_sink() {
  local profile
  profile="$(used_profile)"
  # shellcheck disable=SC2086
  compose $profile --file "$(mesail_root)/deploy/floci/compose.relay-sink.yaml" up --detach --no-deps relay-sink
  touch "$(mesail_log_dir)/relay-sink-started"
}

wait_for_listing() {
  if mesail_wait_for "the relay sink listing on $(listing_url)" 60 curl --fail --silent --output /dev/null "$(listing_url)"; then
    echo "127.0.0.1:$MESAIL_RELAY_SINK_PORT" > "$(mesail_log_dir)/relay-sink-address"
    return 0
  fi
  echo "warning: the relay sink listing does not answer on $(listing_url), so FlowMailOutRelay_test.go skips (dev host: is the port forwarded?)"
}

main() {
  mesail_load_settings
  export MESAIL_RELAY_SINK_PORT="${MESAIL_RELAY_SINK_PORT:-$(mesail_stack .relaySink.listingPort)}"
  export MESAIL_RELAY_SINK_REFUSED_LOCAL_PART="${MESAIL_RELAY_SINK_REFUSED_LOCAL_PART:-$(mesail_stack .relaySink.refusedLocalPart)}"
  MESAIL_RELAY_SINK_DKIM_RECORDS="$(dkim_records)"
  export MESAIL_RELAY_SINK_DKIM_RECORDS
  echo "relay sink: DKIM records for $(jq -r 'keys | join(", ")' <<< "$MESAIL_RELAY_SINK_DKIM_RECORDS")"
  mesail_step relay-sink-up start_sink
  mesail_step relay-sink-ready wait_for_listing
}

main "$@"
