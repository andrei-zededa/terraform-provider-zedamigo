# One big edge-node with many container app-instances

A single QEMU edge-node — **80 vCPUs, 256GB RAM, 1TB disk** — running **27
container edge-app-instances**: a mix of Kafka, Elasticsearch, Kibana, Redis and
PostgreSQL.

It combines the two examples it is derived from: the container edge-app / image /
container-registry-datastore side comes from
[`1_node_1_container`](../1_node_1_container/), the big QEMU edge-node and the
local NAT network-instance come from [`1_node_1_vm`](../1_node_1_vm/).

The point of the example is not to bring up a working data platform (see the
[caveats](#caveats) below) but to put a **known, exact amount of configured load**
on one edge-node, so that Zedcloud resource views, EVE-OS behaviour with many
app-instances, `tofu` run times, etc. can be looked at against a number that is
written down.

## The resource budget

| | |
| --- | --- |
| app-instances | 27 |
| vCPU | 38 |
| RAM | 61 GB |
| storage | **43 GB** = 10 GB persistent volumes + 33 GB dedicated volumes |

Per role:

| role | replicas | image | vCPU each | RAM each | persistent vol. | dedicated vol. |
| ------------------------ | -------: | ---------------------- | --------: | -------: | --------------: | -------------: |
| `kafka_broker`           |        3 | `apache/kafka:3.9.1`   |         2 |   4096MB |           512MB |         3072MB |
| `kafka_controller`       |        3 | `apache/kafka:3.9.1`   |         1 |   1024MB |           256MB |          256MB |
| `elasticsearch_master`   |        3 | `elasticsearch:8.17.4` |         1 |   2048MB |           256MB |          256MB |
| `elasticsearch_data`     |        3 | `elasticsearch:8.17.4` |         2 |   6144MB |           512MB |         2560MB |
| `elasticsearch_ingest`   |        1 | `elasticsearch:8.17.4` |         2 |   4096MB |           256MB |          512MB |
| `kibana`                 |        2 | `kibana:8.17.4`        |         2 |   2048MB |           256MB |          128MB |
| `redis_cache`            |        4 | `redis:7.4-alpine`     |         1 |   1024MB |           256MB |          512MB |
| `redis_sentinel`         |        2 | `redis:7.4-alpine`     |         1 |    512MB |           128MB |          128MB |
| `postgres_primary`       |        2 | `postgres:17-alpine`   |         2 |   3072MB |          1024MB |         2048MB |
| `postgres_replica`       |        4 | `postgres:17-alpine`   |         1 |    768MB |           384MB |         2048MB |
| **total**                |   **27** |                        |    **38** | **61GB** |       **10GB**  |      **33GB**  |

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

plus the singletons: brand, model, project, 2 datastores, 5 images, the
edge-node, one local NAT network-instance which all 27 app-instances share, and
the generated PostgreSQL password. 95 resources in total.

### Why one edge-app definition per *replica* and not per *role*

Normally you would define 5 edge-apps and deploy 27 instances of them. That does
not work here, for two independent reasons:

1. **RAM cannot be set per instance.** `vminfo` of a `zedcloud_application_instance`
   can override `cpus`, but its `memory` attribute is read-only — RAM always
   comes from the edge-app manifest `resources`. Replicas that differ in RAM
   therefore have to differ in their edge-app definition.
2. **Volume labels are unique per edge-node.** A manifest drive claims a volume
   by *label*, and Zedcloud resolves a label to at most one volume-instance per
   edge-node. All 27 replicas land on the *same* edge-node, so each one needs its
   own label — and the label is part of the manifest.

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

All 27 app-instances have a single interface on one local (NAT-ed)
network-instance on the `uplink` (= `eth0`) port. Its subnet is pinned to
`10.166.0.0/16` — a /24 would fit 27 instances, but there is no reason to be
tight in an example about scaling instance counts up.

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
- **27 image pulls.** The edge-node downloads 5 distinct container images, the
  Elasticsearch one alone is well over 1GB, so give the first apply time.
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

## Usage

```
tofu init
tofu apply -parallelism=2
tofu output RESOURCE_BUDGET
tofu output EDGE_APP_INSTANCES
tofu output -json POSTGRES_CREDENTIALS
```

The low `-parallelism` is deliberate: creating the 27 app-instances concurrently
races Zedcloud's per-edge-node VNC display allocator, see the last of the
[caveats](#caveats). `tofu destroy` is worth running with the same flag, so that
every app-instance releases its slot cleanly.

`ZEDEDA_CLOUD_URL` / `ZEDEDA_CLOUD_TOKEN` have to be provided (e.g. via
`TF_VAR_…` environment variables) and `edge_node_ssh_pub_key` / `config_suffix`
via a `terraform.tfvars`.
