# One big edge-node with many container app-instances

A single QEMU edge-node — **80 vCPUs, 256GB RAM, 1TB disk, 10 management
ports** — running **27 container edge-app-instances**: a mix of Kafka,
Elasticsearch, Kibana, Redis and PostgreSQL, **every replica pinned to its own
image version** so that the node downloads 27 distinct images.

It combines the two examples it is derived from: the container edge-app / image /
container-registry-datastore side comes from
[`1_node_1_container`](../1_node_1_container/), the big QEMU edge-node and the
local NAT network-instance come from [`1_node_1_vm`](../1_node_1_vm/).

The point of the example is not to bring up a working data platform (see the
[caveats](#caveats) below). It does two things:

1. puts a **known, exact amount of configured load** on one edge-node, so that
   Zedcloud resource views, EVE-OS behaviour with many app-instances, `tofu`
   run times, etc. can be looked at against a number that is written down;
2. **reproduces the EVE-OS downloader crash** — a mass deployment whose image
   downloads blow the EVE-OS pubsub message-size limit, upon which EVE-OS
   reboots the node mid-deployment. See
   [the section below](#reproducing-the-downloader-crash) for the
   mechanism and what to watch for.

## Reproducing the downloader crash

The scenario this config reproduces: a node deploying 27 app-instances
"crashed and rebooted". The root cause (validated against the EVE-OS
`16.0.1-lts` sources) is a chain in the `pillar` management plane:

1. the `downloader` agent keeps one metrics entry **per unique blob URL** it
   downloads — every image layer, manifest and config blob is one URL — in
   `AgentMetrics.URLCounters`, and nothing ever prunes that map within a boot;
2. every ~10s it publishes the whole map as **one pubsub message**
   (`downloader/MetricsMap`, key `global`);
3. the pubsub socket driver caps a message at **65535 bytes**
   (`pkg/pillar/pubsub/socketdriver/driver.go`) and an oversize publish is
   `log.Fatalf` (`publish.go`) — i.e. **fatal**;
4. all pillar agents live in one `zedbox` process, so the fatal kills the whole
   management plane, and the watchdog then **reboots the node**, recording the
   reason first.

So the trigger is not the number of app-instances and not resource pressure —
it is the number of **unique blobs downloaded within one boot session**
(~250 is where the map crosses the limit, situations when a node had accumulated
> 300 have been seen). That dictates the two load-bearing choices of this config:

- **27 distinct image tags** (`image_tags` in [`workloads.tf`](./workloads.tf)):
  EVE-OS dedups blobs by sha256, so 27 replicas of 5 images dedup to ~60 unique
  blobs and nothing happens — which is exactly why an earlier version of this
  example did not crash. Versions of the same repo released far apart share
  almost no layers: the current catalog resolves to **313 unique blobs**
  (verified against the docker.elastic.co / public.ecr.aws manifests, 2026-08;
  only 43 of 356 blob references dedup away), which serializes to
  **~87KB — 134% of the limit**. The limit is crossed at blob ~235, so the
  fatal hits **mid-download**.

- **the node keeps its uplink**: the earlier `DISABLE_SLIRP_NIC` barrier that
  cut `eth0` after onboarding is gone — with it, the images never downloaded
  and the map never grew.
- **registries the lab can actually reach**: at a previous point in time
  `auth.docker.io` was not reachable from the lab, so anonymous Docker Hub pulls
  died at the manifest step and the downloader records nothing — the second
  silent way this reproduction once failed. Now the catalog pulls from
  `docker.elastic.co` and `public.ecr.aws/docker/library` instead, both
  verified end-to-end from the lab; see [`datastores.tf`](./datastores.tf).

What to expect on apply, in order:

1. node onboards, all 27 app-instances land on it, downloads start;
2. minutes into the download phase (link-speed dependent — ~2 minutes on a
   fast lab link, where the metrics map went 17KB → 33KB → 50KB in
   consecutive 30-second samples), the node **reboots on its own**; Zedcloud
   briefly shows it unreachable, then it comes back;
3. after the reboot the already-verified blobs are reused from `/persist`, the
   remaining downloads finish with a near-empty metrics map, and the
   app-instances come up — the same "crashed, rebooted, then recovered".

To confirm the crash was *the* crash, SSH into EVE-OS
(`tofu output EDGE_NODE_SSH_PORT`, user `root`) after the reboot:

```
cat /persist/log/reboot-reason.log
```

should contain a line like:

```
Reboot from agent downloader[NNNN] in partition IMGA at EVE version
16.0.1-lts-kvm-amd64 at <timestamp>: fatal: agent downloader[NNNN]:
Too large message (NNNNN bytes) sent to downloader/MetricsMap topic MetricsMap key global
```

To watch it coming, `eve exec pillar` and check the size of the downloader
metrics map growing while the pulls run; or simply watch
`/persist/newlog/collect/` for the `Too large message` fatal.

## The resource budget

| | |
| --- | --- |
| app-instances | 27 |
| vCPU | 38 |
| RAM | 61 GB |
| storage | **43 GB** = 10 GB persistent volumes + 33 GB dedicated volumes |

Per role — note that **each replica pins its own image tag** (first replica
gets the first tag and so on), which is what produces the 27 distinct images
the [crash reproduction](#reproducing-the-downloader-crash) needs:

| role | replicas | image tags (one per replica) | vCPU each | RAM each | persistent vol. | dedicated vol. |
| ------------------------ | -------: | ------------------------------------------------ | --------: | -------: | --------------: | -------------: |
| `logstash_pipeline`      |        3 | `logstash`: 8.17.4, 8.15.5, 8.14.3                |         2 |   4096MB |           512MB |         3072MB |
| `logstash_agent`         |        3 | `logstash`: 8.13.4, 8.11.4, 7.17.10               |         1 |   1024MB |           256MB |          256MB |
| `elasticsearch_master`   |        3 | `elasticsearch`: 8.17.4, 8.15.5, 8.14.3           |         1 |   2048MB |           256MB |          256MB |
| `elasticsearch_data`     |        3 | `elasticsearch`: 8.13.4, 8.12.2, 8.11.4           |         2 |   6144MB |           512MB |         2560MB |
| `elasticsearch_ingest`   |        1 | `elasticsearch`: 7.17.10                          |         2 |   4096MB |           256MB |          512MB |
| `kibana`                 |        2 | `kibana`: 8.17.4, 8.15.5                          |         2 |   2048MB |           256MB |          128MB |
| `redis_cache`            |        4 | `redis`: 7.4, 7.2, 7.0, 6.2                       |         1 |   1024MB |           256MB |          512MB |
| `redis_sentinel`         |        2 | `redis`: 6.0, 5.0                                 |         1 |    512MB |           128MB |          128MB |
| `postgres_primary`       |        2 | `postgres`: 17, 16                                |         2 |   3072MB |          1024MB |         2048MB |
| `postgres_replica`       |        4 | `postgres`: 15, 14, 13, 12                        |         1 |    768MB |           384MB |         2048MB |
| **total**                |   **27** | 27 distinct tags, ~313 unique blobs               |    **38** | **61GB** |       **10GB**  |      **33GB**  |

Elasticsearch, Kibana and Logstash come from `docker.elastic.co`, Redis and
PostgreSQL from the `docker/library` mirror on `public.ecr.aws` (Debian-based
variants, not `-alpine`: more layers per image, i.e. more unique blobs). The
two `logstash_*` roles ran `apache/kafka` when this example pulled from Docker
Hub; Kafka has no anonymously-pullable home on the registries this lab can
reach, and Logstash keeps the catalog inside the Elastic product family.

All of this lives in one place, `local.WORKLOAD_ROLES` in
[`workloads.tf`](./workloads.tf), and a `check` block in the same file asserts
that the map still adds up to the numbers above. `tofu output RESOURCE_BUDGET`
prints the totals, `tofu output WORKLOAD_BREAKDOWN` the per-role table.

## How the config is generated

`local.WORKLOAD_ROLES` is expanded into `local.WORKLOADS`, a flat map with one
entry per replica keyed `<role>_<n>` (`kafka_broker_1`, `kafka_broker_2`, …).
Three resources then `for_each` over it:

```
local.WORKLOADS ──> zedcloud_application          (edge_apps.tf)          27x
                ──> zedcloud_volume_instance      (edge_app_instances.tf) 27x
                ──> zedcloud_application_instance (edge_app_instances.tf) 27x
```

plus the **27 images** (`zedcloud_image.CONTAINER`, one per distinct
`repo:tag` of the catalog, `for_each` over `local.CONTAINER_IMAGE_SET` in
[`images.tf`](./images.tf)) and the singletons: brand, model, project,
2 datastores, the edge-node, one local NAT network-instance which all 27
app-instances share, and the generated PostgreSQL password — and the nine
`zedamigo_netns` / `zedamigo_tap` / `zedamigo_dhcp_server` triplets of the
extra management ports, which `for_each` over `local.EXTRA_MGMT_PORTS` in the
same way. 148 resources in total.

### Why one edge-app definition per *replica* and not per *role*

Normally you would define 5 edge-apps and deploy 27 instances of them. That does
not work here, for three independent reasons:

1. **RAM cannot be set per instance.** `vminfo` of a `zedcloud_application_instance`
   can override `cpus`, but its `memory` attribute is read-only — RAM always
   comes from the edge-app manifest `resources`. Replicas that differ in RAM
   therefore have to differ in their edge-app definition.
2. **Volume labels are unique per edge-node.** A manifest drive claims a volume
   by *label*, and Zedcloud resolves a label to at most one volume-instance per
   edge-node. All 27 replicas land on the *same* edge-node, so each one needs its
   own label — and the label is part of the manifest.
3. **Every replica pins its own image version.** The image reference is part of
   the manifest too, and 27 distinct images is the precondition for the
   [downloader crash](#reproducing-the-downloader-crash) this example
   reproduces.

## The two kinds of volume

Each of the 27 manifests declares three drives, and the difference between the
last two is what produces the `10 GB + 33 GB` split of the storage budget:

| drive | mountpath | volume label | size from | created by |
| ----- | --------- | ------------ | --------- | ---------- |
| `[0]` container image | `/` | – | – | Zedcloud, implicit read-only content-tree volume |
| `[1]` persistent volume | the role's `persist_path`, i.e. `/mnt/persist` | – | manifest `maxsize` | Zedcloud, implicit blank block-storage volume |
| `[2]` dedicated volume | the role's `data_path`, `/mnt/data` except for PostgreSQL | `<workload>_data_<suffix>` | the volume-instance | Terraform, `zedcloud_volume_instance` |

Drive `[1]` has neither a volume label nor an image name, which is exactly what
makes Zedcloud create a **blank implicit block-storage volume** of `maxsize` for
every app-instance (the same mechanism as in
[`1_node_1_vm_proj_policies`](../1_node_1_vm_proj_policies/)). These volumes are
*not* in the Terraform state; Zedcloud creates and deletes them together with the
app-instance. They are mounted at the `persist_path` of the role and add up to
**10 GB**.

Drive `[2]` does have a volume label, so Zedcloud looks for a volume-instance
with that label on the edge-node — one of the 27
`zedcloud_volume_instance.APP_DEDICATED_DATA` resources, which are dedicated
(`multiattach = false`) to exactly one app-instance each. They are mounted at the
`data_path` of the role and add up to **33 GB**.

### The volumes are mostly not written to

With one exception the two volumes are mounted at `/mnt/persist` and `/mnt/data`,
paths which none of the images use. That is deliberate, and it is worth
understanding why before "improving" it:

- A volume mount **hides** whatever the image ships at that path. This is the
  decisive one: mounting an empty volume over `/usr/share/elasticsearch/config`
  or `/usr/share/kibana/config` removes `elasticsearch.yml` / `kibana.yml` and
  the container cannot start at all. The first version of this example did
  exactly that.
- Several of these images run as a non-root user (`elasticsearch` is uid 1000,
  `apache/kafka` runs as `appuser`). If the mount is root-owned such a container
  cannot create anything inside it, so pointing e.g. `KAFKA_LOG_DIRS` at the
  volume only moves the failure. A freshly formatted volume also normally
  contains `lost+found`, which upsets anything insisting on an empty directory
  (`initdb` does). Neither of these was verified against EVE-OS here — they are
  the reason the safe path was chosen rather than measured facts.

The exception is PostgreSQL, whose entrypoint starts as root and chowns its data
directory itself, so it can cope with both of the above. Its dedicated volume is
mounted at `/var/lib/postgresql/data` and `PGDATA` is set to the `pgdata`
**subdirectory** of it, which keeps `initdb` away from `lost+found` and lets the
entrypoint fix the ownership.

For the other roles the volumes exist to occupy the storage budget, which is what
this example measures. Making one of them genuinely usable means either an image
that starts as root, or an init step that chowns the mount.

### Container environment

Roles whose image needs configuration in order to start carry it in the `env` map
of `local.WORKLOAD_ROLES`. For a **container** app-instance EVE-OS does not run
cloud-init: it parses the user-data and turns every `KEY=VALUE` entry of the
cloud-config `runcmd` list into an environment variable of the container, so this
is how `docker run --env KEY=VALUE` is expressed. Keys may contain dots, which is
what makes `discovery.type=single-node` work.

| role | environment | why |
| ---- | ----------- | --- |
| `postgres_*` | `POSTGRES_PASSWORD`, `POSTGRES_DB`, `PGDATA` | the image **refuses to initialize** without a password, and an app-instance whose container exits takes the whole app VM down (`the init-initrd is about to quit by calling /sbin/poweroff`) |
| `elasticsearch_*` | `discovery.type`, `xpack.security.enabled`, `ES_JAVA_OPTS` | single-node puts ES in development mode so the `vm.max_map_count` bootstrap check (a host sysctl) is skipped; the heap has to be pinned or the JVM sizes it from the edge-node's 256GB and gets OOM-killed |
| `kafka_*`, `kibana`, `redis_*` | – | these images come up with no configuration |

`POSTGRES_PASSWORD` is a generated `random_password`; `tofu output -json
POSTGRES_CREDENTIALS` prints it.

Unlike [`1_node_1_container`](../1_node_1_container/) this uses no Zedcloud
custom-config *variables*. Terraform renders the final user-data with
`yamlencode`, so there is nothing left for Zedcloud to substitute, `field_delimiter`
is empty, and the edge-app-**instances** carry no `custom_config` block at all —
with `override = false` Zedcloud takes the template from the edge-app definition,
and variable groups on an instance only exist to supply values for substitution.

Watch the units, they differ per field:

- manifest `maxsize` → **kilobytes**
- manifest `resources { name = "memory" }` → **kilobytes**, as a `"%.2f"` string
- `zedcloud_volume_instance.size_bytes` → **bytes**

The container image drives contribute **0** to the storage numbers: for a
container image Zedcloud cannot know the size upfront, so `image_size_bytes` is
0 and the implicit content-tree volumes it creates are reported as 0 bytes.

## Networking and reachability

### Ten management ports, one of which matters

| port | backed by | addressing | `cost` | reaches |
| ---- | --------- | ---------- | -----: | ------- |
| `eth0` | QEMU user-mode networking ("SLIRP"), i.e. the default `nic0` | QEMU's internal DHCP + NAT | 0 | the controller and the internet |
| `eth1` … `eth9` | a TAP moved into its own host network namespace | a `zedamigo_dhcp_server` on `10.99.N.0/24`, pool `.70`–`.79`, gateway `10.99.N.1` on the TAP itself | 1 … 9 | its own gateway, nothing past it |

All ten are `ADAPTER_USAGE_MANAGEMENT`. The nine extra ones are generated from
`local.EXTRA_MGMT_PORTS` in [`host_networking.tf`](./host_networking.tf), which
is the single source for the four places that have to agree about them: the
`io_member_list` of the model (Zedcloud rejects an edge-node interface for a
port the model does not declare), the `interfaces` of the edge-node, the QEMU
NICs in `extra_qemu_args`, and the netns / TAP / DHCP-server triplets themselves.

Nine extra NICs do not fit the usual `-nic tap,…` spelling: `-nic` creates a
*board* NIC and a QEMU machine has only eight such slots (`MAX_NICS`), one of
which `nic0` already holds, so the eighth TAP fails with `no more
on-board/default NIC slots available`. `extra_qemu_args` therefore builds
`-netdev` + `-device virtio-net-pci` pairs on a `pci-bridge` — 32 slots — behind
a `pcie-root-port`, and pins each NIC to the bridge slot matching its port
index, which is what makes the Nth port `ethN` in the guest. Board NICs are
realized before any `-device`, so `eth0` stays `eth0` regardless.

**The nine extra ports are deliberately dead ends.** A `zedamigo_netns` holds
nothing but the TAP that was moved into it: there is no veth back to the root
namespace, no forwarding, no NAT, and this provider has no resource that would
add any of them. The TAP does carry `10.99.N.1/24`, the same address the DHCP
server advertises as the router, so `ethN` completes DHCP, installs `10.99.N.1`
as a default route, and that gateway answers — it is the host end of the link.
One hop further there is nothing: the namespace has no other interface to
forward to. That is the same shape as [`1_node_1_vm`](../1_node_1_vm/), and
here it is the whole point: nine ports that look usable to EVE-OS and are not.

The `cost` values are what make that arrangement observable rather than merely
broken. EVE-OS uses the cheapest usable management port, so while `eth0` is up
at cost 0 it carries everything — including all 27 image pulls, which the
[crash reproduction](#reproducing-the-downloader-crash) depends on —
and the other nine sit there holding leases.

An earlier version of this example took `eth0` down over QMP right after
onboarding (a `zedamigo_wait_until` barrier issuing
`{"execute":"set_link","arguments":{"name":"usernet0","up":false}}`), to watch
EVE-OS attempt a failover across the nine dead ends. That barrier is gone:
cutting the uplink stops the image pulls, and no pulls means no crash. The QMP
socket path is still exported (`tofu output -raw EDGE_NODE_QMP_SOCKET`) if you
want to simulate a carrier loss by hand — `set_link` with `"up": true` puts the
link back, the NIC is never unplugged and the guest port numbering never
shifts.

### The app-instance network

All 27 app-instances have a single interface on one local (NAT-ed)
network-instance on the `uplink` port. Its subnet is pinned to `10.166.0.0/16` —
a /24 would fit 27 instances, but there is no reason to be tight in an example
about scaling instance counts up.

`uplink` is a *shared* port label meaning "the management ports", so it no
longer resolves to `eth0` alone the way it did when this example had a single
NIC — but with `eth0` the only cost-0 port and the only one with a path
anywhere, that is where it lands.

Each app-instance also port-maps its service port to its own port on the
edge-node, handed out from `10080` upwards in the alphabetical order of the
workload names (`tofu output EDGE_APP_INSTANCES` prints the mapping). Only the
first one is reachable from the host without extra work, because `zedamigo`
forwards host port `<ssh_port> + 2` to edge-node port `10080`:

```
tofu output EDGE_APP_LOCALHOST_PORT
```

For the rest, either SSH into EVE-OS (`tofu output EDGE_NODE_SSH_PORT`) or use
edgeview.

## Caveats

- **The node reboots itself mid-apply — that is the point.** Expect Zedcloud to
  show the node unreachable for a few minutes somewhere in the download phase,
  and the `Too large message … downloader/MetricsMap` line in
  `/persist/log/reboot-reason.log` afterwards; see
  [the reproduction section](#reproducing-the-downloader-crash). The
  node recovers on its own: verified blobs survive in `/persist`, so the
  post-reboot session finishes the remaining downloads with a near-empty
  metrics map. If it ever crash-*loops* instead, that just means the first
  session got very few blobs downloaded — it will converge after the next
  reboot for the same reason.
- **Registry throttling shows up as content-tree errors, not failed applies.**
  All 27 images are pulled anonymously. `docker.elastic.co` imposes no pull
  limits; `public.ecr.aws` throttles anonymous request *bursts* per IP with
  `429`s, which EVE-OS retries with backoff — the redis/postgres downloads may
  simply lag behind the Elastic ones. If an app-instance sits in an error
  state naming a `429` or `Too Many Requests`, that is what it is.
- **The containers start, but they do not form clusters.** Each replica is
  configured to come up *standalone*; nothing points the Kibanas at an
  Elasticsearch, makes the Kafka nodes share a KRaft quorum or makes a
  `postgres_replica` stream from a `postgres_primary`. The role names describe the
  shape of the deployment the resource budget is modelled on, not a working data
  platform.
- **The host has to be big.** With the defaults of `vars.tf` the QEMU VM asks
  for 80 vCPUs and 256GB of RAM. Either run this on a machine that size or point
  the `zedamigo` provider at a remote host over SSH — there is a commented-out
  block in [`terraform.tf`](./terraform.tf) showing the form `1_node_1_vm` uses.
  `EDGE_NODE_CPUS` / `EDGE_NODE_MEM_GB` / `EDGE_NODE_DISK_MB` can be lowered to
  try the config out on something smaller; the 27 app-instances need 38 vCPU,
  61GB and 43GB of volumes plus whatever EVE-OS itself takes.
- **27 image pulls, all distinct.** The edge-node downloads 27 distinct
  container images — roughly 10–15GB, the Elasticsearch and Kibana ones alone
  are ~1GB each — so give the first apply time. This is deliberate; making
  replicas share images again is exactly what stops the crash from
  reproducing, and the `check` block in `workloads.tf` warns if the catalog
  regresses to fewer distinct tags than app-instances.
- **There is a hard limit of 63 app-instances per edge-node, and VNC being
  disabled does not help.** Zedcloud allocates a *VNC display number* out of a
  per-edge-node pool of 63 for **every** app-instance it creates, whether or not
  the edge-app has VNC enabled — the allocation is not gated on the manifest
  `enablevnc` flag. Running out of that pool is what produces:

  ```
  cannot allocate VNC display number for instance: <name>
  ```

  27 instances fit comfortably in 63, so seeing this error means the pool of that
  edge-node has been *leaked into*, not filled legitimately. Two things cause
  that, and both are provoked by creating this many app-instances at once:

  - a slot is taken before the app-instance row is written, and is only given
    back when an app-instance is *deleted* — so every app-instance create that
    fails half-way (and every one Terraform then retries) leaks a slot
    permanently;
  - the allocation is a read-modify-write of a bitmap on the edge-node row,
    guarded by a lock that does not span the database write, so parallel creates
    against the same edge-node can lose each other's updates.

  Mitigations, in order of preference:

  1. Apply with reduced parallelism so the 27 app-instances are not created
     concurrently — see [Usage](#usage) below. This avoids the race.
  2. The pool is stored on the edge-node object and is only reset when that
     object is *created*, so once slots have leaked the only way to get them back
     is to recreate the edge-node in Zedcloud:
     `tofu apply -replace=zedcloud_edgenode.ENODE_TEST`. Note that this cascades:
     the network-instance, all 27 volume-instances and all 27 app-instances
     reference the edge-node id and will be recreated too.
  3. Delete leftover app-instances of previous runs properly (`tofu destroy`)
     rather than abandoning them, so their slots are released.
- **On a remote target, the nine netns/TAP/DHCP triplets can exhaust the sshd
  session budget.** Everything the provider does on a remote `target` goes over
  one SSH connection: each command is a session channel on it and SFTP holds one
  more permanently. OpenSSH counts those against `MaxSessions`, default **10**
  per connection, so a default `-parallelism=10` can ask for eleven and the
  eleventh is refused:

  ```
  Error: TAP Resource Error
    with zedamigo_tap.MGMT["eth4"],
  Failed to start TAP mover daemon: failed to start TAP mover:
  ssh: rejected: connect failed (open failed)
  ```

  It lands on whichever resource loses the race, so it looks arbitrary and moves
  around between runs, and running the very same command by hand on the target
  works — which is what makes it confusing. `-parallelism=2` avoids it; so does
  raising `MaxSessions` on the target. Jump hosts are irrelevant here: they carry
  a single `direct-tcpip` channel, which is not counted. Check the target with
  the `maxsessions` line of `sshd -T`; sshd logs `no more sessions` when it
  refuses one.

## Usage

```
tofu init
tofu apply -parallelism=2
tofu output RESOURCE_BUDGET
tofu output CONTAINER_IMAGE_CATALOG
tofu output EDGE_APP_INSTANCES
tofu output -json POSTGRES_CREDENTIALS
tofu output EXTRA_MGMT_PORTS
```

The apply finishes once the Zedcloud objects exist; the interesting part —
onboarding, 27 image downloads, the downloader crash and the self-reboot —
happens on the node afterwards. Watch it via `tofu output EDGE_NODE_SSH_PORT`
+ SSH, the serial console log of the VM, or the edge-node events in Zedcloud;
see [the reproduction section](#reproducing-the-downloader-crash)
for what to look for.

The low `-parallelism` is deliberate, and now for two independent reasons:
creating the 27 app-instances concurrently races Zedcloud's per-edge-node VNC
display allocator, and creating the nine netns/TAP/DHCP triplets concurrently
can exhaust the SSH session budget of a remote target — see the last two
[caveats](#caveats). `tofu destroy` is worth running with the same flag, so that
every app-instance releases its slot cleanly.

`ZEDEDA_CLOUD_URL` / `ZEDEDA_CLOUD_TOKEN` have to be provided (e.g. via
`TF_VAR_…` environment variables) and `edge_node_ssh_pub_key` / `config_suffix`
via a `terraform.tfvars`.
