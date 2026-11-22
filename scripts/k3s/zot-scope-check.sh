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

auth_json() {
  local user="$1" secret="$2"
  printf '{"auths":{"127.0.0.1:%s":{"auth":"%s"}}}\n' "$(pin registry.host_port)" \
    "$(printf '%s:%s' "$user" "$(secret_password "$secret")" | base64)"
}

destination() {
  echo "docker://127.0.0.1:$(pin registry.host_port)/$1"
}

push_with_login() {
  local image="$1" user="$2" secret="$3" target="$4"
  skopeo copy --dest-tls-verify=false --src-daemon-host "$DOCKER_HOST" \
    --dest-authfile <(auth_json "$user" "$secret") \
    "docker-daemon:$image" "$(destination "$target")" > /dev/null 2>&1
}

push_without_login() {
  local image="$1" target="$2"
  skopeo copy --dest-tls-verify=false --src-daemon-host "$DOCKER_HOST" --dest-no-creds \
    "docker-daemon:$image" "$(destination "$target")" > /dev/null 2>&1
}

expect() {
  local want="$1" label="$2" result=denied
  shift 2
  if "$@"; then
    result=allowed
  fi
  echo "$label: $result (expected $want)"
  [ "$result" = "$want" ]
}

run_checks() {
  local image="$1" tag
  tag="scope-$(date +%Y%m%d%H%M%S)"
  expect allowed "paas-pusher -> paas-develop/zot-scope-check" push_with_login "$image" paas-pusher paas-pusher "paas-develop/zot-scope-check:$tag"
  expect denied "paas-pusher -> mesail/zot-scope-check" push_with_login "$image" paas-pusher paas-pusher "mesail/zot-scope-check:$tag"
  expect allowed "pusher -> mesail/zot-scope-check" push_with_login "$image" "$(pin registry.pusher)" zot-pusher "mesail/zot-scope-check:$tag"
  expect denied "anonymous -> paas-develop/zot-scope-check" push_without_login "$image" "paas-develop/zot-scope-check:$tag"
  echo "zot-scope-check: all expectations met"
}

main() {
  eval "$(mesail-devhost env)"
  run_checks "${1:-mesail/node-check-probe:local}"
}

main "$@"
