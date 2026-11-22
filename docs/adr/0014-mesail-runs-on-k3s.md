# ADR 0014: Mesail runs on K3s

- Status: Accepted (2026-11-21, the owner, quoted through the hub:
  "for Mesail i think we should use K3s ... so that when we can
  comfortable deploy to kestrel with the same tools with confidence";
  then "actually let mimic kestrel, no Floci. run k3s here as you would
  in kestrel"; and on scope "Yes, all to K3s (Rec.)").
- Date: 2026-11-21
- Supersedes: [ADR 0007](0007-on-premise-first.md) section 4 (Lambda
  images in Floci on kestrel) and the "Packaging" and "One writer per
  (service, domain)" Lambda parts of
  [ADR 0004](0004-mesail-go-serverless-keycloak.md) section 4. Every other
  part of ADR 0004 and ADR 0007 stays, including the on-premise rules,
  the FYLO layout and one writer per FYLO root.
- Slices: MSL-K41 (this ADR and the dev node), K42 (images to Zot), K43
  (manifests and the `dev` overlay), K44 (MSL-INT on K3s), K45 (switch-over
  and Floci retirement on the dev host), K46 (the `kestrel` overlay).
- Sources: PaaS `apps/k3s-pilot` (kestrel's K3s pilot, read at commit
  `925e679a`: `deployment.json`, `registry.json`, `flux.json`,
  `guest_pilot.py`, `README.md`); FYLO ADR 0033 section 5.1 (a root's
  parent is a mount point); the FYLO Build Coordinator's answer on leases
  (2026-11-21).

## Context

ADR 0007 section 4 chose Lambda images in Floci on kestrel (the owner's
answer to SPEC Q18 on 2026-11-11). Since then kestrel changed:

- kestrel runs no Floci. Its staging sites left Floci S3, and its
  production snippet runner moved from a Floci Lambda to K3s on
  2026-11-19. Its production Colima runs no Floci (checked 2026-11-21).
- kestrel's tool set for containers is now its K3s pilot (PaaS
  `apps/k3s-pilot`): K3s `v1.37.2+k3s1` from airgap files, Zot as the
  image registry inside K3s, skopeo to push, and Flux with kustomize to
  deploy from git. Every image is named by digest.

So "Lambda images in Floci on kestrel" no longer describes a platform
that exists. The owner wants this Mac to run Mesail the same way kestrel
will, so that the kestrel deploy uses tools already proven here.

Lambdas were a choice made for Floci. Without Floci, no Mesail part needs
the Lambda form: one long-running process per FYLO root already gives
one writer per root, and `realtime` already runs as a long-running
process.

## Direction added 2026-11-21: PaaS's `develop` branch as the development platform

The owner, quoted through the hub: "that `develop` branch in `PaaS` repo is
looking real nice about now. we could use it for develop branches these
projects" and "use it stictly for development, it does not merge to
staging". So the K3s node on this Mac is meant to come from PaaS's
`develop` branch (its k3s-pilot node setup, Zot, Flux and kustomize files),
used only for development and never merged into PaaS `staging`, rather
than from a separate Mesail-only setup. The decisions below (K3s, Zot,
skopeo, Flux, kustomize, Deployments, FYLO roots) stay; only where the
node's files live may move to PaaS.

The owner added: "the develop branch is the same as the staging, but for
local development (localhost) and NEVER exposed to the internet". So:

- **The branch rule.** PaaS gets a new `develop` branch, made from
  `staging`, strictly for local development on this Mac. It is never
  merged into `staging` or `production`, and nothing on it is deployed to
  kestrel. The PaaS Build Coordinator owns the branch; Mesail never
  changes PaaS itself.
- **Localhost only** (section 7): the K3s API, Zot and every Service
  answer only on loopback, and nothing has an ingress to a public address,
  a Tailscale name or a public DNS name.

Open (2026-11-21, MSL-K41): the PaaS repository has no `develop` branch
yet, on GitHub or in any checkout on this Mac; its long-lived branches are
`production` and `staging`. A PaaS Build Coordinator will create `develop`
from `staging`. Until then, `deploy/k3s/` in Mesail holds the node files,
copied from the pilot at `925e679a`; `apps/k3s-pilot` on `staging`
(`6e6aa3b3d`) is the same (its newer commits change only runbooks, the
README and a snippet test).

## Decision

### 1. Every Mesail part runs on K3s, on this Mac and on kestrel

Every part that runs in the Floci stack today moves to K3s: the Yon
apps, the router, the schedulers, `realtime`, the SMTP edge and
Keycloak. No Lambda remains.

- **FYLO-owning apps** (directory, channels, messaging, attachments,
  notify, search, mail-in, mail-out, devicebox, realtime) are each a
  Kubernetes Deployment with `replicas: 1` and `strategy: Recreate`. Recreate
  stops the old pod before the new one starts, so a root never has two
  writers during a rollout. Each runs `ty start` with
  `YON_PROFILE=container`. The AWS Lambda Web Adapter is not used.
- **One writer per root** (ADR 0004 section 4) holds as before. FYLO's
  lease is 20 s; fencing stops an old pod with `EROOTLEASELOST` before it
  commits. Each FYLO-owning pod closes FYLO cleanly on SIGTERM
  (`TAC_SHUTDOWN_GRACE_SECONDS`) and has `terminationGracePeriodSeconds`
  of 30 or more.
- **Plain Deployments, not Knative.** Knative starts a new pod before the
  old one stops, and scale to zero reopens FYLO cold. Knative may come
  later for `identity` alone (no FYLO root), by a new decision.
- **The router** runs in its proxy mode and sends each service prefix to
  that app's cluster Service. Its Lambda mode and its per-function queue
  are not used.
- **Schedulers** are Deployments with the same image (a CronJob cannot
  tick every 30 s). Keycloak is a Deployment (one copy, Recreate) with its
  SQLite file on a local volume on the node's own disk, never on NFS
  (OPERATIONS.md section 7).
- **Test-only parts** (the relay sink, test DKIM keys, test people) live
  only in the kustomize `dev` overlay. The `kestrel` overlay never names
  them.

### 2. The node: K3s as on kestrel's pilot

| Setting | Value (the same on this Mac and on kestrel) |
| --- | --- |
| K3s | `v1.37.2+k3s1`, arm64, from the airgap files: `k3s-arm64` SHA-256 `a779ccf1e5e2e33a62f5a29c2f1c0bbf99431e100579d4c10f0aa0742a6aa84b`, `k3s-airgap-images-arm64.tar.zst` SHA-256 `ca384f1a77aa83024bd82ad26fa4774971adb038e656a3b5b9af926be445a442` |
| Datastore | SQLite (K3s default), one server node |
| Turned off | Traefik and ServiceLB (`disable: [traefik, servicelb]`) |
| Secrets | `secrets-encryption: true` |
| Kubeconfig | `write-kubeconfig-mode: "0600"` |
| API certificate | `tls-san: [127.0.0.1]` |
| Volumes | `default-local-storage-path` on a data disk, never the VM's OS disk |
| Unit | `k3s.service`, `ExecStartPre=mountpoint -q` for each disk it needs |
| Service range guard | the pilot's nftables rule that rejects untranslated `10.43.0.0/16` traffic before it leaves the VM |
| Network policies | every workload namespace denies outgoing traffic; each app gets only its declared hosts (ADR 0007 section 2) |

Every file is checked against its pinned SHA-256 before use. A file
whose checksum differs stops the install.

### 3. Images: Zot inside K3s, pushed with skopeo, pulled by digest

- **Zot `v2.1.23`** (`ghcr.io/project-zot/zot-linux-arm64@sha256:14006bb4ca46e1bc250f3fc227b850fe1f7a9f5a751869af684f3b65cdb2c713`)
  runs as Deployment `registry/zot`, set up as the pilot's
  `registry.json`: namespace pod-security `restricted`, user 10002,
  read-only root, no service-account token, one copy with Recreate,
  a `ClusterIP` Service (kestrel: NodePort 30500), dedupe, garbage collection (delay 1 h, every 24 h), a
  daily scrub. Anyone in the cluster may pull; only the user `pusher` may
  push; nobody may delete. The push password is made once inside the
  cluster, kept in a Secret and a 0600 auth file outside the repository,
  and never printed.
- **Reach:** a `kubectl port-forward --address 127.0.0.1 service/zot
  30500:5000` unit (the pilot's Headlamp pattern) puts Zot on the guest's
  loopback, and Lima forwards that to the host's loopback only,
  `127.0.0.1:15000` (the same host port as on kestrel). Nothing on the LAN
  reaches it.
- **Push:** `skopeo copy docker-daemon:<image> docker://127.0.0.1:<port>/mesail/<app>:<tag>`
  from the host, with the pusher's auth file.
- **Pull:** `/etc/rancher/k3s/registries.yaml` sends
  `registry.mesail.local/...` to Zot inside the node. Every manifest names
  an image by digest (`registry.mesail.local/mesail/<app>@sha256:<digest>`),
  never by tag. A check fails on a manifest that names a tag (K42).
- **Retention:** keep the last 2 pushed digests of each image; untagged
  images go after a day (see section 8 for why 2, not kestrel's 3).
- **Run time never downloads** (ADR 0007 section 3): the node pulls only
  from Zot inside the install.

### 4. Deploys: kustomize overlays applied by Flux

- **Manifests:** plain Kubernetes YAML with kustomize, in
  `deploy/k3s/base/` plus overlays `dev` (this Mac) and `kestrel`
  (production). No Helm.
- **Flux `v2.10.0`**, as the pilot: only the source and kustomize
  controllers, from the pinned `flux-install.yaml` (SHA-256
  `a8c838ccd9be5d30eba9df87cd229b7c4eb80c0049fba1bc3b9840829f267f28`)
  with the controller images pinned by digest
  (`ghcr.io/fluxcd/source-controller@sha256:611b6bca3eae95bcff471414d1fb14a4ab83d7f14e609cccef2bd389c121f37e`,
  `ghcr.io/fluxcd/kustomize-controller@sha256:37db29b25dae35b881a5e064acedb0cc954442da453a39eaa0eab7a90dbecc4e`). Each workload namespace has a
  least-privilege deployer service account. A merge into the deploy
  branch is the deploy.
- `kubectl apply -k` is allowed only for the node check, and on this Mac
  for testing a branch before it is merged (the `scripts/k3s/` test
  scripts, K43). Everything else goes through Flux.

### 5. FYLO roots: one mount per app, the root one folder below it

FYLO refuses a root whose parent is not a mount point (FYLO ADR 0033
section 5.1; `ENOTVOLUMEROOT`), and refuses a root on the node's boot
disk (`ESYSTEMDISK`). So:

- The data area holds one folder per app: `<area>/<app>.vol/<app>`.
- Each FYLO-owning pod mounts only `<area>/<app>.vol`, through a volume
  with `subPath: <app>.vol`, at `/mnt/data`. The bind mount makes
  `/mnt/data` a mount point, and the app's root is `/mnt/data/<app>`.
  Each pod sees only its own app.
- Never a `hostPath` on the node's boot disk.
- On this Mac the area is the dev host's ext4 data disk on HANGAR. On
  kestrel it is the Mesail-only area on ATLAS: `/Volumes/MESAIL/<app>`
  (ADR 0004 section 4) becomes `/Volumes/MESAIL/<app>.vol/<app>` (K46),
  opened by FYLO as a network root with leases on a `hard` NFS mount
  (needs a FYLO build with L1-5, DB-1 or later).
- The blob folder stays one shared folder (ADR 0007 section 7), mounted
  by attachments, mail-in, mail-out, the router and the SMTP edge.

### 6. What does not change

- On-premise only, no cloud account, no hidden outbound traffic, run time
  never downloads (ADR 0007 sections 1 to 3, 5 and 8). The Floci rows in
  ADR 0007 section 5 now read: Lambda per service is a Deployment per
  service; the router serves the web bundles from its own image or a
  volume, not from Floci's S3; EFS is a subPath mount on the data area.
- One writer per FYLO root, the FYLO layout, bounded work per call, the
  outbox and the scheduler (ADR 0004 section 4, EVENTS.md).
- Native `ty start` runs (`scripts/native/run-all.sh`) stay the way every
  slice is tested.

### 7. Localhost only on this Mac

The development node is never exposed to the internet or the LAN:

- **Services:** every Service is `ClusterIP`. No `NodePort`, no
  `LoadBalancer`, no Ingress, no Tailscale, no public DNS name.
- **The host:** the only ports on the Mac are Lima forwards to
  `127.0.0.1` (Zot's 15000 and the existing Mesail ports). A Service the
  Mac must reach gets a `kubectl port-forward --address 127.0.0.1` unit in
  the guest and a Lima forward to `127.0.0.1`.
- **The K3s API:** not forwarded to the Mac; reached only with
  `mesail-devhost exec sudo k3s kubectl ...`. It is not bound to the
  guest's loopback itself, because the pods (Flux, CoreDNS) reach it
  through the node's address. Instead the guard
  (`node/service-cidr-guard.nft`) rejects the API, kubelet, kube-proxy,
  NodePort and flannel ports arriving on the VM's network interface
  `eth0`, so they answer only inside the VM.
- **The check:** `scripts/k3s/node-check.sh` fails when a Service is not
  `ClusterIP`, when the Mac's port 15000 listens on anything but
  `127.0.0.1`, or when the guard is missing.

### 8. Where this Mac differs from kestrel, and why

| Part | kestrel's pilot | This Mac | Why |
| --- | --- | --- | --- |
| VM | its own Lima VM `paas-k3s-pilot`, 4 vCPU, 4 GiB | the existing `mesail` dev host (Lima, vz), 6 vCPU, 8 GiB | a third VM breaks the Mac's 2-VM cap; the dev host already has the `--vm=mesail` lock (Platform's choice) |
| Node name | `paas-k3s-pilot` | `mesail` | the dev host's name |
| Images and local volumes | K3s state and images in `/var/lib/rancher/k3s` on the VM's OS disk; local-path on the pilot's data disk on ATLAS | `/var/lib/rancher/k3s` bound by the dev host supervisor onto a new sparse image disk on HANGAR (30 GiB cap, mounted by UUID at `/mnt/mesail-k3s`); local-path and FYLO roots on the existing data disk | development never uses ATLAS or HAVEN; the dev host's OS disk is already 76% full; HANGAR holds the VMs and virtual disks |
| Start at boot | `k3s.service` enabled at boot | `k3s.service` installed but never enabled; the dev host supervisor starts it after the image disk is mounted and bound | a K3s started before the bind writes to the VM's OS disk, the bind is then refused (the folder is not empty) and the whole dev host is blocked |
| Zot's store | its own 64 GiB disk on HAVEN | the HANGAR image disk | the same rule |
| Zot retention | the 3 most recently pushed tags | the last 2 digests | HANGAR is small (about 29 GiB free on 2026-11-21) |
| Mirror name | `registry.paas.local` | `registry.mesail.local` | Mesail's own images; the `kestrel` overlay sets kestrel's name (K46) |
| K3s API | forwarded to `127.0.0.1:16443` on kestrel | not forwarded; answers only inside the VM (the guard rejects 6443 on `eth0`); reached with `mesail-devhost exec sudo k3s kubectl ...` | other sessions on this Mac must not reach the cluster outside the dev host's lock; localhost only (section 7) |
| Zot Service | NodePort 30500, forwarded by Lima | `ClusterIP`, put on the guest loopback by a port-forward unit, forwarded by Lima to `127.0.0.1:15000` | no NodePort on the development node (section 7) |
| Reach | the private front door and Tailscale serve chosen hosts (`lamp.del.ma`, `store.del.ma`) | nothing is served beyond this Mac: the only host ports are Lima forwards to `127.0.0.1` (Zot 15000 and the existing Mesail ports); the K3s API, kubelet and NodePorts stay inside the VM, whose Lima network the LAN cannot reach | the owner: the development platform is "for local development (localhost) and NEVER exposed to the internet" |
| Flux source | the PaaS repository over SSH with a deploy key titled `kestrel-k3s-pilot-flux` | the Mesail repository over SSH with its own read-only deploy key titled `mesail-k3s-falcos-mini-2 flux read-only` (never a pilot title, never made by PaaS's deploy-key script, which deletes keys by title); branch named by the `dev` overlay | Mesail's manifests live in Mesail (K46 decides PaaS's pointer with Platform) |
| Knative, Kourier, Headlamp | installed | not installed | Mesail uses plain Deployments (section 1); Headlamp may come later from the pilot's pin |
| Floci | none | keeps running beside K3s until K45 | today's blackbox suite stays green until K3s passes the same suite |
| Data area | ATLAS `/Volumes/MESAIL` (NFS) | the ext4 data disk on HANGAR | tests never use ATLAS |

## Consequences

- The kestrel deploy (K46) uses the same K3s version, settings, registry,
  push tool and deploy tool that every Mesail slice uses on this Mac.
- Floci leaves Mesail: after K45 nothing in Mesail runs Floci. The Floci
  rules of ADR 0007 section 4 (its own VM, its API never reachable, the
  egress container) are replaced by K3s network policies that deny
  outgoing traffic by default.
- The dev host grows from 6 to 8 GiB and gets a new image disk on HANGAR.
  It counts as one of the two VMs that may run at once, so it is taken
  down when no Mesail work needs it.
- New downloads, each pinned by SHA-256 or digest: the K3s files (copied
  from kestrel's checked cache), Zot, the two Flux controllers and skopeo.
- The Lambda parts of the images (the Lambda Web Adapter, the entrypoint
  that writes `.env` per function, the router's Lambda mode, the
  per-function queue) are removed after K45. Until then they stay, so the
  Floci stack keeps working.
- Docs that still describe Lambda in Floci (`CLAUDE.md`, SPEC's hosting
  line, OPERATIONS, `deploy/floci/README.md`, SLICES rule texts) are
  updated in K45, when the switch-over happens.
