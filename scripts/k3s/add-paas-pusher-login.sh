#!/usr/bin/env bash
set -euo pipefail

registry_kubectl() {
  k3s kubectl -n registry "$@"
}

secret_value() {
  registry_kubectl get secret "$1" -o "jsonpath={.data.$2}" | base64 --decode
}

bcrypt_hash() {
  python3 -W ignore -c 'import crypt,sys; print(crypt.crypt(sys.stdin.read().strip(), crypt.mksalt(crypt.METHOD_BLOWFISH)))'
}

main() {
  local current hash
  current=$(secret_value zot-htpasswd htpasswd)
  hash=$(secret_value paas-pusher password | bcrypt_hash)
  printf '%s\npaas-pusher:%s\n' "$current" "$hash" \
    | registry_kubectl create secret generic zot-htpasswd --from-file=htpasswd=/dev/stdin --dry-run=client -o yaml \
    | registry_kubectl apply -f -
}

main "$@"
