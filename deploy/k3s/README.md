# Mesail on K3s

Mesail runs on K3s, set up like kestrel's K3s pilot (PaaS `apps/k3s-pilot`).
The decision and every difference from kestrel are in
[ADR 0014](../../docs/adr/0014-mesail-runs-on-k3s.md).

## Files

| Path | What it is |
| --- | --- |
| `node/pins.json` | the pinned versions, SHA-256s and image digests (from the pilot's `deployment.json`) |
| `node/config.yaml` | `/etc/rancher/k3s/config.yaml`: the pilot's settings, node name `mesail` |
| `node/registries.yaml` | `registry.mesail.local` -> Zot's NodePort inside the node |
| `node/k3s.service` | the pilot's unit, with a mount check for each disk |
| `node/var-lib-rancher.mount` | binds `/var/lib/rancher` onto the image disk, so images never fill the OS disk |
| `node/service-cidr-guard.*` | the pilot's nftables guard for `10.43.0.0/16` |
| `registry/` | Zot as the pilot's `registry.json`, keeping the last 2 digests per image |
| `node-check/` | the FYLO probe: a pod mounts `apps/nodecheck.vol` by subPath at `/mnt/data` and writes and reads the root `/mnt/data/nodecheck` |

## Disks in the dev host

| Guest path | Disk | Holds |
| --- | --- | --- |
| `/mnt/mesail-k3s` | the image disk on HANGAR (`/Volumes/HANGAR/mesail/k3s/mesail-k3s-images.raw`, 25 GiB cap, mounted by UUID) | `rancher/` (K3s state and images), `local-path/`, `registry/` (Zot) |
| `/mnt/mesail-data` | the existing data disk on HANGAR | `apps/<app>.vol/<app>` (FYLO roots), Floci's Docker volumes until K45 |

## What Platform sets up first

- The dev host's memory from 6 to 8 GiB.
- The image disk, mounted at `/mnt/mesail-k3s` by UUID, with the supervisor's guards.
- A Lima forward: guest port 30500 -> `127.0.0.1:15000` on the Mac (the same host port as on kestrel).

## Order

1. `scripts/k3s/stage-node-files.sh`: copies the K3s files and `flux-install.yaml` from kestrel's cache and checks every SHA-256.
2. `scripts/k3s/install-node.sh`: installs K3s, Zot and Flux in the running dev host.
3. `scripts/k3s/node-check.sh --probe-image <image with fylo>`: K3s ready, Zot answers, a test image pushed with skopeo and pulled by digest, the FYLO root probe, Flux, and memory and disk numbers.

Use the cluster only through `mesail-devhost exec sudo k3s kubectl ...`. The K3s API is not forwarded to the Mac.
