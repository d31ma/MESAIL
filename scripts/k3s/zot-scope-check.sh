#!/usr/bin/env bash
# Usage: scripts/k3s/zot-scope-check.sh [docker image]   (default mesail/node-check-probe:local)
set -Eeuo pipefail
trap 'echo "=== step failed rc=$? at line $LINENO $(date +%H:%M:%S)"' ERR

scope_root() {
  cd "$(dirname "$0")/../.." && pwd
}

pin() {
  python3 -I -c 'import json,sys; d=json.load(open(sys.argv[1]))
for k in sys.argv[2].split("."): d=d[k]
print(d)' "$(scope_root)/deploy/k3s/node/pins.json" "$1"
}

secret_password() {
  mesail-devhost exec sudo k3s kubectl -n registry get secret "$1" -o 'jsonpath={.data.password}' | base64 --decode
}

write_auth() {
  local user="$1" secret="$2" file="$3"
  ( umask 077; printf '{"auths":{"127.0.0.1:%s":{"auth":"%s"}}}\n' "$(pin registry.host_port)" \
      "$(printf '%s:%s' "$user" "$(secret_password "$secret")" | base64)" > "$file" )
}

push() {
  local image="$1" auth="$2" target="$3"
  skopeo copy --dest-tls-verify=false --src-daemon-host "$DOCKER_HOST" "$auth" \
    "docker-daemon:$image" "docker://127.0.0.1:$(pin registry.host_port)/$target" > /dev/null 2>&1
}

expect() {
  local want="$1" label="$2" result=denied
  shift 2
  if push "$@"; then
    result=allowed
  fi
  echo "$label: $result (expected $want)"
  [ "$result" = "$want" ]
}

run_checks() {
  local folder="$1" image="$2" tag
  tag="scope-$(date +%Y%m%d%H%M%S)"
  write_auth "$(pin registry.pusher)" zot-pusher "$folder/pusher.json"
  write_auth paas-pusher paas-pusher "$folder/paas.json"
  expect allowed "paas-pusher -> paas-develop/zot-scope-check" "$image" "--dest-authfile=$folder/paas.json" "paas-develop/zot-scope-check:$tag"
  expect denied "paas-pusher -> mesail/zot-scope-check" "$image" "--dest-authfile=$folder/paas.json" "mesail/zot-scope-check:$tag"
  expect allowed "pusher -> mesail/zot-scope-check" "$image" "--dest-authfile=$folder/pusher.json" "mesail/zot-scope-check:$tag"
  expect denied "anonymous -> paas-develop/zot-scope-check" "$image" "--dest-no-creds" "paas-develop/zot-scope-check:$tag"
  echo "zot-scope-check: all expectations met"
}

main() {
  local folder
  eval "$(mesail-devhost env)"
  folder=$(mktemp -d)
  # shellcheck disable=SC2064
  trap "rm -rf '$folder'" EXIT
  run_checks "$folder" "${1:-mesail/node-check-probe:local}"
}

main "$@"
