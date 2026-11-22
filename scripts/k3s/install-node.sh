#!/usr/bin/env bash
# Installs the Mesail K3s node in the running dev host, as kestrel's pilot
# (ADR 0014). It never enables k3s.service at boot: the dev host
# supervisor starts it after the image disk is mounted and
# /var/lib/rancher/k3s is bound onto it. Needs: the user's download yes, Platform's OK, the dev host
# at 8 GiB with the image disk mounted at /mnt/mesail-k3s and the Lima
# forward of guest 127.0.0.1:30500 -> 127.0.0.1:15000, and the files staged by
# stage-node-files.sh. It never touches Floci's containers or volumes.
# Usage: scripts/k3s/install-node.sh
set -euo pipefail

install_root() {
  cd "$(dirname "$0")/../.." && pwd
}

stage_dir() {
  echo "${MESAIL_K3S_STAGE:-$HOME/Library/Caches/mesail-k3s}"
}

guest() {
  mesail-devhost exec "$@"
}

pin() {
  python3 -I -c 'import json,sys; d=json.load(open(sys.argv[1]))
for k in sys.argv[2].split("."): d=d[k]
print(d)' "$(install_root)/deploy/k3s/node/pins.json" "$1"
}

step() {
  local name="$1" rc=0
  shift
  echo "=== $name $(date +%H:%M:%S)"
  "$@" || rc=$?
  echo "=== $name rc=$rc $(date +%H:%M:%S)"
  return "$rc"
}

put_file() {
  local source="$1" target="$2" mode="$3"
  guest sudo install -d -m 0755 "$(dirname "$target")"
  guest sudo tee "$target.part" > /dev/null < "$source"
  guest sudo chmod "$mode" "$target.part"
  guest sudo mv "$target.part" "$target"
}

require_mounts() {
  guest mountpoint -q "$(pin disks.images)"
  guest mountpoint -q "$(pin disks.data)"
}

guest_sha_matches() {
  [ "$(guest sudo sha256sum "$1" | cut -d' ' -f1)" = "$2" ]
}

prepare_folders() {
  guest sudo install -d -m 0755 /mnt/mesail-data/local-path
  guest sudo install -d -m 0750 -o 10002 -g 10002 /mnt/mesail-k3s/registry
  guest sudo install -d -m 0755 "$(pin disks.app_area)"
  guest sudo install -d -m 0750 -o 10001 -g 10001 "$(pin disks.app_area)/nodecheck.vol"
}

install_units() {
  local node
  node="$(install_root)/deploy/k3s/node"
  put_file "$node/service-cidr-guard.nft" /etc/mesail-service-cidr-guard.nft 0644
  put_file "$node/service-cidr-guard.service" /etc/systemd/system/mesail-service-cidr-guard.service 0644
  put_file "$node/k3s.service" /etc/systemd/system/k3s.service 0644
  put_file "$node/zot-forward.service" /etc/systemd/system/zot-forward.service 0644
  guest sudo systemctl daemon-reload
  guest sudo systemctl enable --now mesail-service-cidr-guard.service
  guest mountpoint -q /var/lib/rancher/k3s
}

install_k3s_files() {
  local node stage
  node="$(install_root)/deploy/k3s/node"
  stage="$(stage_dir)"
  put_file "$stage/k3s-arm64" /usr/local/bin/k3s 0755
  guest_sha_matches /usr/local/bin/k3s "$(pin 'k3s.files.k3s-arm64')"
  put_file "$stage/k3s-airgap-images-arm64.tar.zst" /var/lib/rancher/k3s/agent/images/k3s-airgap-images-arm64.tar.zst 0644
  guest_sha_matches /var/lib/rancher/k3s/agent/images/k3s-airgap-images-arm64.tar.zst "$(pin 'k3s.files.k3s-airgap-images-arm64.tar.zst')"
  put_file "$node/config.yaml" /etc/rancher/k3s/config.yaml 0600
  put_file "$node/registries.yaml" /etc/rancher/k3s/registries.yaml 0644
  guest sudo ln -sfn /usr/local/bin/k3s /usr/local/bin/kubectl
}

k3s_not_enabled() {
  [ "$(guest systemctl is-enabled k3s.service 2>/dev/null || true)" != enabled ]
}

start_k3s() {
  k3s_not_enabled || { echo "install-node: k3s.service is enabled at boot; the dev host supervisor must start it" >&2; return 1; }
  guest sudo systemctl start k3s.service
  guest sudo k3s kubectl wait --for=condition=Ready "node/$(pin node_name)" --timeout=300s
}

zot_auth_file() {
  echo "${MESAIL_ZOT_AUTH:-$HOME/.config/mesail/k3s/zot-auth.json}"
}

pusher_secret_exists() {
  guest sudo k3s kubectl -n registry get secret zot-pusher > /dev/null 2>&1
}

create_pusher() {
  local password hash
  password=$(openssl rand -hex 24)
  hash=$(guest python3 -W ignore -c 'import crypt,sys; print(crypt.crypt(sys.stdin.read().strip(), crypt.mksalt(crypt.METHOD_BLOWFISH)))' <<< "$password")
  guest sudo k3s kubectl create namespace registry --dry-run=client -o yaml | guest sudo k3s kubectl apply -f -
  printf '%s:%s\n' "$(pin registry.pusher)" "$hash" | guest sudo k3s kubectl -n registry create secret generic zot-htpasswd --from-file=htpasswd=/dev/stdin
  printf '%s' "$password" | guest sudo k3s kubectl -n registry create secret generic zot-pusher --from-file=password=/dev/stdin
}

write_auth_file() {
  local password auth
  auth="$(zot_auth_file)"
  password=$(guest sudo k3s kubectl -n registry get secret zot-pusher -o 'jsonpath={.data.password}' | base64 --decode)
  mkdir -p "$(dirname "$auth")"
  ( umask 077; printf '{"auths":{"127.0.0.1:%s":{"auth":"%s"}}}\n' "$(pin registry.host_port)" \
      "$(printf '%s:%s' "$(pin registry.pusher)" "$password" | base64)" > "$auth" )
  echo "pusher auth file: $auth (0600, password not printed)"
}

install_registry() {
  pusher_secret_exists || create_pusher
  write_auth_file
  guest sudo k3s kubectl apply -f - < "$(install_root)/deploy/k3s/registry/zot.yaml"
  guest sudo k3s kubectl -n registry rollout status deployment/zot --timeout=300s
  guest sudo systemctl enable zot-forward.service
  guest sudo systemctl restart zot-forward.service
}

install_flux() {
  local pinned
  pinned=$(python3 -I "$(install_root)/scripts/k3s/flux_pinner.py" "$(install_root)/deploy/k3s/node/pins.json" "$(stage_dir)/flux-install.yaml")
  guest sudo k3s kubectl apply --server-side -f - <<< "$pinned"
  guest sudo k3s kubectl -n flux-system wait --for=condition=Available deployment/source-controller deployment/kustomize-controller --timeout=300s
}

main() {
  step mounts require_mounts
  step folders prepare_folders
  step units install_units
  step k3s-files install_k3s_files
  step k3s-start start_k3s
  step registry install_registry
  step flux install_flux
  echo "install-node: done; run scripts/k3s/node-check.sh"
}

main "$@"
