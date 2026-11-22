#!/usr/bin/env bash
# Copies the pinned K3s and Flux files from kestrel's checked cache
# (~/Library/Caches/PaaS/k3s-pilot/<sha256>-<name>) over the Thunderbolt
# bridge to this Mac's stage folder, and checks every SHA-256 against
# deploy/k3s/node/pins.json. Run only after the user's download yes.
# Usage: scripts/k3s/stage-node-files.sh
set -euo pipefail

stage_root() {
  cd "$(dirname "$0")/../.." && pwd
}

stage_dir() {
  echo "${MESAIL_K3S_STAGE:-$HOME/Library/Caches/mesail-k3s}"
}

pinned_files() {
  python3 -I -c 'import json,sys; d=json.load(open(sys.argv[1]))
for group in ("k3s","flux"):
    for name, sha in d[group]["files"].items(): print(name, sha)' "$(stage_root)/deploy/k3s/node/pins.json"
}

file_matches() {
  [ -f "$1" ] && [ "$(shasum -a 256 "$1" | cut -d' ' -f1)" = "$2" ]
}

copy_one() {
  local name="$1" sha="$2" target
  target="$(stage_dir)/$name"
  if file_matches "$target" "$sha"; then
    echo "stage: $name already staged ($sha)"
    return
  fi
  echo "=== copy $name $(date +%H:%M:%S)"
  scp -q "kestrel:Library/Caches/PaaS/k3s-pilot/$sha-$name" "$target.part"
  file_matches "$target.part" "$sha" || { rm -f "$target.part"; echo "stage: $name differs from its pinned SHA-256" >&2; exit 1; }
  mv "$target.part" "$target"
  echo "=== copy $name rc=0 $(date +%H:%M:%S)"
}

main() {
  mkdir -p "$(stage_dir)"
  pinned_files | while read -r name sha; do copy_one "$name" "$sha"; done
  echo "stage: all files in $(stage_dir) match their pins"
}

main "$@"
