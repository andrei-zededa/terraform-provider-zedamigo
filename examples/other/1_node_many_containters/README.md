# One big edge-node with many container app-instances

A single QEMU edge-node — **80 vCPUs, 256GB RAM, 1TB disk, 10 management
ports** — running **27 container edge-app-instances**: a mix of Kafka,
Elasticsearch, Kibana, Redis and PostgreSQL. Once the node has reported in, a
barrier resource takes the link of its only working uplink down over QMP and
leaves it with nine dead-end management ports; see
[Networking and reachability](#networking-and-reachability).

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
the generated PostgreSQL password — and the nine `zedamigo_netns` /
`zedamigo_tap` / `zedamigo_dhcp_server` triplets of the extra management ports,
which `for_each` over `local.EXTRA_MGMT_PORTS` in the same way. 127 resources in
total.

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

### Ten management ports, one of which works

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
at cost 0 it carries everything and the other nine sit there holding leases.

### Pulling the only working port out

`zedamigo_wait_until.DISABLE_SLIRP_NIC` in [`edge_nodes.tf`](./edge_nodes.tf)
runs after the edge-node VM has started. Its script is a single-shot probe —
the resource owns the retrying — that does two things:

1. Asks Zedcloud for the node's `runState`
   (`GET /api/v1/devices/status-config`, filtered by device name, the record
   picked out by id) and fails the attempt unless it is `RUN_STATE_BOOTING` or
   `RUN_STATE_ONLINE`. Those are the first states that mean EVE-OS is actually
   running and talking to the controller, as opposed to `RUN_STATE_PROVISIONED`
   (or `ADMIN_STATE_REGISTERED`), which only mean the object exists.
2. Connects to the VM's QMP socket and takes the user-mode NIC's link down:

   ```json
   {"execute":"set_link","arguments":{"name":"usernet0","up":false}}
   ```

`usernet0` is the id `zedamigo` gives the default `nic0` netdev, and `set_link`
on a netdev propagates to the NIC peered with it, so from inside EVE-OS this is
a carrier loss on `eth0`. The NIC is **not** unplugged: the interface stays, the
guest's port numbering does not shift, and the same command with `"up": true`
puts it back. To do that by hand, on the provider target:

```
printf '%s\n%s\n' '{"execute":"qmp_capabilities"}' \
  '{"execute":"set_link","arguments":{"name":"usernet0","up":true}}' \
  | socat -t 5 - "UNIX-CONNECT:$(tofu output -raw EDGE_NODE_QMP_SOCKET)"
```

The probe runs **on the provider `target`**, which is the reason to use
`zedamigo_wait_until` rather than a `local-exec` provisioner: the QMP socket
only exists next to the QEMU process, so with a remote `target` anything running
locally would be looking in the wrong place. It needs `curl`, `jq` and `socat`
installed there and says so if they are missing.

### The app-instance network

All 27 app-instances have a single interface on one local (NAT-ed)
network-instance on the `uplink` port. Its subnet is pinned to `10.166.0.0/16` —
a /24 would fit 27 instances, but there is no reason to be tight in an example
about scaling instance counts up.

`uplink` is a *shared* port label meaning "the management ports", so it no
longer resolves to `eth0` alone the way it did when this example had a single
NIC: after the barrier has run, the network-instance can be moved onto one of
the nine dead ends.

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

- **The node goes offline, on purpose, and does not come back on its own.**
  `eth0` is the only port with a path to the controller, so once
  `DISABLE_SLIRP_NIC` has taken its link down the node stops reporting and
  Zedcloud will show it offline (after a failover attempt across the nine
  `10.99.N.0/24` ports, which is the thing worth watching). Nothing in the
  config brings it back — put the link up again by hand with the `set_link …
  "up": true` command [above](#pulling-the-only-working-port-out).
- **The barrier fires long before the images are pulled.** It waits for
  `RUN_STATE_BOOTING`, which EVE-OS reaches within a minute or two of booting,
  and the 27 app-instances need several GB of container images off the internet.
  If you want the workloads to actually come up first, wait for
  `RUN_STATE_ONLINE` only, or add a second condition to the probe. As written,
  expect the app-instances to be stuck downloading.
- **The Zedcloud API token ends up on the target.** `zedamigo_wait_until` has no
  way to pass a secret separately from the script, so `var.ZEDEDA_CLOUD_TOKEN`
  is interpolated into `script` — which means it is in the Terraform state and
  in `<lib_path>/wait_until/<id>/wait_until.sh` on the provider target. Terraform
  does propagate the variable's `sensitive` flag, so it is redacted in plan
  output and marked sensitive in state, but it is not encrypted at rest in
  either place. Use a token you are willing to leave there.
- **The probe needs `curl`, `jq` and `socat` on the provider target** — not on
  the machine running `tofu`, unless they are the same machine. A missing tool
  is reported once per attempt until `timeout` expires, because the resource has
  no notion of a fatal exit, and the message includes the `PATH` the script
  searched. `zedamigo_wait_until` injects no `PATH`, so the script *adds* the
  usual directories to whatever the target's non-interactive shell handed it
  rather than overwriting it — overwriting breaks targets which keep their tools
  somewhere else entirely, e.g. NixOS' `/run/current-system/sw/bin`, where the
  tools are plainly there but `PATH=/usr/bin:...` cannot see them.
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
tofu output EDGE_APP_INSTANCES
tofu output -json POSTGRES_CREDENTIALS
tofu output EXTRA_MGMT_PORTS
tofu output SLIRP_NIC_DISABLED
```

The apply blocks on `zedamigo_wait_until.DISABLE_SLIRP_NIC` until the node
reports in to Zedcloud, which is most of an EVE-OS boot. A provider cannot
stream output the way `local-exec` does, so run with `TF_LOG=info` to watch the
attempts go by; the full per-attempt output stays under
`<lib_path>/wait_until/<id>/attempts/` on the target either way.

The low `-parallelism` is deliberate, and now for two independent reasons:
creating the 27 app-instances concurrently races Zedcloud's per-edge-node VNC
display allocator, and creating the nine netns/TAP/DHCP triplets concurrently
can exhaust the SSH session budget of a remote target — see the last two
[caveats](#caveats). `tofu destroy` is worth running with the same flag, so that
every app-instance releases its slot cleanly.

`ZEDEDA_CLOUD_URL` / `ZEDEDA_CLOUD_TOKEN` have to be provided (e.g. via
`TF_VAR_…` environment variables) and `edge_node_ssh_pub_key` / `config_suffix`
via a `terraform.tfvars`.
