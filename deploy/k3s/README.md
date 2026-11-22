# Mesail on K3s

Mesail runs on K3s, set up like kestrel's K3s pilot (PaaS `apps/k3s-pilot`).
The decision and every difference from kestrel are in
[ADR 0014](../../docs/adr/0014-mesail-runs-on-k3s.md).

## Files

| Path | What it is |
| --- | --- |
| `node/pins.json` | the pinned versions, SHA-256s and image digests (from the pilot's `deployment.json`) |
| `node/config.yaml` | `/etc/rancher/k3s/config.yaml`: the pilot's settings, node name `mesail` |
| `node/registries.yaml` | `registry.mesail.local` -> Zot on the guest loopback (`127.0.0.1:30500`) |
| `node/zot-forward.service` | `kubectl port-forward --address 127.0.0.1 service/zot 30500:5000`, as the pilot's Headlamp forward |
| `node/k3s.service` | the pilot's unit, with a mount check for each disk; installed but never enabled at boot |
| `node/service-cidr-guard.*` | the pilot's nftables guard for `10.43.0.0/16`, plus an input rule that rejects the K3s ports (API 6443, kubelet, kube-proxy, NodePorts, flannel) arriving on the VM's network interface `eth0` |
| `registry/` | Zot as the pilot's `registry.json`, keeping the last 2 digests per image |
| `node-check/` | the FYLO probe: a pod mounts `apps/nodecheck.vol` by subPath at `/mnt/data` and writes and reads the root `/mnt/data/nodecheck` |

## Disks in the dev host

| Guest path | Disk | Holds |
| --- | --- | --- |
| `/mnt/mesail-k3s` | the image disk on HANGAR (`/Volumes/HANGAR/mesail/k3s/mesail-k3s-images.raw`, 30 GiB cap, mounted by UUID); the dev host supervisor binds `/var/lib/rancher/k3s` onto `/mnt/mesail-k3s/rancher/k3s` | K3s state and images, `registry/` (Zot) |
| `/mnt/mesail-data` | the existing data disk on HANGAR | `local-path/`, `apps/<app>.vol/<app>` (FYLO roots), Floci's Docker volumes until K45 |

## K3s never starts at boot

Install K3s with `k3s.service` left disabled (as `INSTALL_K3S_SKIP_ENABLE=true` does).
The dev host supervisor starts `k3s.service` at each VM start, only after the image disk
is mounted and `/var/lib/rancher/k3s` is bound onto it. If K3s started at boot, it would
write to the VM's own disk; the next bind is then refused (the folder is not empty) and
the whole dev host is blocked. `install-node.sh` stops when it finds the unit enabled.

## What the dev host session sets up first

- The dev host's memory from 6 to 8 GiB.
- The image disk, mounted at `/mnt/mesail-k3s` by UUID, with the supervisor's guards and the bind of `/var/lib/rancher/k3s` (staged in `~/.local/share/mesail-devhost-staged/k3sdisk/APPLY.md`).
- A Lima forward: guest `127.0.0.1:30500` -> `127.0.0.1:15000` on the Mac (the same host port as on kestrel).

## Order

1. `scripts/k3s/stage-node-files.sh`: copies the K3s files and `flux-install.yaml` from kestrel's cache and checks every SHA-256.
2. `scripts/k3s/install-node.sh`: installs K3s, Zot and Flux's controllers (no source) in the running dev host.
3. `scripts/k3s/node-check.sh --probe-image <image with fylo>`: K3s ready, Zot answers, a test image pushed with skopeo and pulled by digest, the FYLO root probe, Flux, and memory and disk numbers.

## What lives here and what lives in PaaS

This folder is the node layer only: K3s in the dev host, `registries.yaml`, the image disk's use, Zot,
the guard and the node check. Flux's source and Mesail's app manifests for this Mac live in PaaS
`develop` (`clusters/mesail-dev/develop`, Mesail's apps in `.../mesail/`), set up by PaaS with its own
read-only deploy key. `install-node.sh` installs Flux's controllers with no source. Never add a
GitRepository for `d31ma/MESAIL` or a deploy key here.

## Localhost only (the owner's rule for PaaS `develop`)

This node is for local development on this Mac and is NEVER exposed to the internet (ADR 0014 section 7):
- Every Mac port is a Lima forward with `hostIP: 127.0.0.1`; no shared or bridged network. PaaS's
  `tools/check_localhost_only.py --live` checks it.
- No `LoadBalancer`, external IPs, public Ingress, Tailscale or public DNS. A NodePort stays inside the VM:
  the guard rejects the K3s ports (API 6443, kubelet, kube-proxy, NodePorts, flannel) on `eth0`.
- Zot is `ClusterIP`, reached through a port-forward on the guest's loopback and the Lima forward to `127.0.0.1:15000`.
- `node-check.sh` fails on a `LoadBalancer` or external-IP Service, on port 15000 listening anywhere but
  `127.0.0.1`, or when the guard is missing.

Use the cluster only through `mesail-devhost exec sudo k3s kubectl ...`. The K3s API is not forwarded to the Mac.
