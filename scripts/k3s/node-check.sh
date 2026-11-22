#!/usr/bin/env bash
# Checks the Mesail K3s node in the dev host (ADR 0014, slice MSL-K41).
# Usage: scripts/k3s/node-check.sh [--probe-image <docker image>]
set -euo pipefail

node_check_root() {
  cd "$(dirname "$0")/../.." && pwd
}

node_check_log_dir() {
  local base
  base="${LOG_ROOT:-$HOME/Library/Logs/local}/MESAIL/k3s-node-check/$(date +%Y%m%d-%H%M%S)"
  mkdir -p "$base"
  echo "$base"
}

pin() {
  python3 -I -c 'import json,sys; d=json.load(open(sys.argv[1]))
for k in sys.argv[2].split("."): d=d[k]
print(d)' "$(node_check_root)/deploy/k3s/node/pins.json" "$1"
}

step_start() {
  echo "=== $1 $(date +%H:%M:%S)"
}

step_end() {
  echo "=== $1 rc=$2 $(date +%H:%M:%S)"
}

guest() {
  mesail-devhost exec "$@"
}

kubectl_guest() {
  guest sudo k3s kubectl "$@"
}

check_node_ready() {
  local status
  status=$(kubectl_guest get node "$(pin node_name)" -o 'jsonpath={.status.conditions[?(@.type=="Ready")].status}')
  echo "node $(pin node_name) Ready=$status, K3s $(guest k3s --version | head -1)"
  [ "$status" = True ]
}

check_registry_answers() {
  local code
  code=$(curl --silent --output /dev/null --write-out '%{http_code}' "http://127.0.0.1:$(pin registry.host_port)/v2/")
  echo "Zot at 127.0.0.1:$(pin registry.host_port)/v2/ answered HTTP $code"
  [ "$code" = 200 ]
}

zot_auth_file() {
  echo "${MESAIL_ZOT_AUTH:-$HOME/.config/mesail/k3s/zot-auth.json}"
}

push_probe() {
  local image="$1" tag digest_file
  tag="check-$(date +%Y%m%d%H%M%S)"
  digest_file=$(mktemp)
  skopeo copy --authfile "$(zot_auth_file)" --dest-tls-verify=false --digestfile "$digest_file" \
    --src-daemon-host "$DOCKER_HOST" \
    "docker-daemon:$image" "docker://127.0.0.1:$(pin registry.host_port)/mesail/node-check-probe:$tag" >&2
  cat "$digest_file"
  rm -f "$digest_file"
}

pull_by_digest() {
  local reference
  reference="$(pin registry.mirror)/mesail/node-check-probe@$1"
  guest sudo k3s crictl pull "$reference"
}

probe_manifest() {
  local digest="${1#sha256:}" mode="$2" marker="$3"
  sed -e "s/PROBE_DIGEST/$digest/" -e "s/PROBE_MODE/$mode/g" -e "s/PROBE_MARKER/$marker/" \
    "$(node_check_root)/deploy/k3s/node-check/fylo-probe.yaml"
}

run_probe_job() {
  local digest="$1" mode="$2" marker="$3"
  probe_manifest "$digest" "$mode" "$marker" | kubectl_guest apply -f -
  kubectl_guest -n node-check wait --for=condition=complete "job/fylo-probe-$mode" --timeout=180s
  kubectl_guest -n node-check logs "job/fylo-probe-$mode"
}

clear_probe_jobs() {
  kubectl_guest -n node-check delete job fylo-probe-write fylo-probe-read --ignore-not-found
}

check_fylo_root() {
  local digest="$1" marker
  marker="k41-$(date +%s)"
  clear_probe_jobs
  run_probe_job "$digest" write "$marker"
  run_probe_job "$digest" read "$marker"
}

check_flux() {
  kubectl_guest -n flux-system wait --for=condition=Available deployment/source-controller deployment/kustomize-controller --timeout=60s
  kubectl_guest -n flux-system get gitrepositories,kustomizations 2>&1 || true
}

report_numbers() {
  guest free -m
  guest df -h / "$(pin disks.images)" "$(pin disks.data)"
  kubectl_guest top node 2>&1 || echo "metrics not ready yet"
  df -h /Volumes/HANGAR
}

run_step() {
  local name="$1" rc=0
  shift
  step_start "$name"
  "$@" || rc=$?
  step_end "$name" "$rc"
  return "$rc"
}

parse_probe_image() {
  if [ "${1:-}" = --probe-image ]; then
    echo "$2"
    return
  fi
  echo "${MESAIL_K3S_PROBE_IMAGE:-mesail/node-check-probe:local}"
}

run_checks() {
  local image="$1" digest
  eval "$(mesail-devhost env)"
  run_step node-ready check_node_ready
  run_step registry check_registry_answers
  step_start push
  digest=$(push_probe "$image")
  echo "pushed $image as $digest"
  step_end push 0
  run_step pull-by-digest pull_by_digest "$digest"
  run_step fylo-root check_fylo_root "$digest"
  run_step flux check_flux
  run_step numbers report_numbers
  echo "node-check: all steps passed"
}

main() {
  local log image
  log="$(node_check_log_dir)/node-check.log"
  image=$(parse_probe_image "$@")
  echo "node-check: log $log"
  run_checks "$image" 2>&1 | tee "$log"
}

main "$@"
