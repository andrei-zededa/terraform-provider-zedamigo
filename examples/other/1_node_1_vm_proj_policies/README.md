# Edge-node with a VM deployed via project app-policy + network-policy

This is a variant of [`1_node_1_vm`](../1_node_1_vm/): the same single QEMU
edge-node, the same host networking and the same Ubuntu 24.04 VM edge-app, but
the network-instances and the edge-app-instance are **not** created by
Terraform. Instead the project carries a **network-policy** and an
**app-policy**, and Zedcloud creates both automatically for every edge-node that
joins the project.

## What changed compared to `1_node_1_vm`

| `1_node_1_vm`                                        | this example                                                       |
| ---------------------------------------------------- | ------------------------------------------------------------------ |
| `zedcloud_network_instance` × 3                       | `network_policy { network_policy { net_instance_config … } }`        |
| `zedcloud_application_instance`                       | `app_policy { app_policy { apps … } }`                               |
| `zedcloud_volume_instance` (persistent data disk)     | implicit volume, created by Zedcloud from the edge-app manifest      |
| cloud-init values set on the edge-app-**instance**    | cloud-init values set on the edge-app **definition**                 |

Everything else (`brands_and_models.tf`, `datastores.tf`, `images.tf`,
`host_networking.tf`, `edge_nodes.tf`, `terraform.tf`, `vars.tf`,
`ubuntu_cloud_init.txt`) is unchanged.

## How the policies fit together

A project policy is always two nested blocks: an outer generic wrapper carrying
`name`/`title`/`type`, and an inner block carrying the payload. The `type` must
match the payload block, otherwise Zedcloud rejects the policy.

```hcl
resource "zedcloud_project" "PROJECT" {
  # …
  network_policy {                     # generic policy wrapper
    type = "POLICY_TYPE_NETWORK"
    network_policy {                   # payload
      net_instance_config { … }
    }
  }

  app_policy {                         # generic policy wrapper
    type = "POLICY_TYPE_APP"
    app_policy {                       # payload
      apps { … }
    }
  }
}
```

When an edge-node is created inside the project Zedcloud runs a single
auto-deployment job which

1. creates one network-instance per `net_instance_config` entry, naming each one
   `<net_instance_config name>.<edge-node name>` and pinning it to that
   edge-node;
2. resolves the interfaces of each app in the app-policy to those
   network-instances;
3. creates the edge-app-instance, naming it according to `naming_scheme`.

Note that this is triggered by the **edge-node object being created in the
project** (i.e. by `zedcloud_edgenode`), not by the physical node onboarding.
The app-instance config therefore already exists in Zedcloud while EVE-OS is
still installing/onboarding, and the node picks it up once it connects.

## Interfaces are matched by tag, not by name

The name of an auto-created network-instance is only known once the edge-node
name is appended to it, so the app-policy cannot reference it by name. Instead:

- `default_net_instance = true` matches whichever network-instance of the
  edge-node has `device_default = true` — that is the local NAT one here;
- `netinsttag = { … }` matches a network-instance of the edge-node whose `tags`
  contain the given map.

```
 app_eth0 ── default_net_instance ──> ni_local_nat_<sfx>.<node>    local NAT, uplink
 app_eth1 ── ni_role = switch_eth1 ──> ni_switch_eth1_<sfx>.<node>  switch, eth1
 app_eth2 ── ni_role = switch_eth2 ──> ni_switch_eth2_<sfx>.<node>  switch, eth2
```

The `netinstname` attribute is required by the provider schema but Zedcloud
overwrites it during auto-deployment; it is filled in here only as
documentation. **If an interface has neither `default_net_instance` nor a
`netinsttag` which matches an existing network-instance the whole
auto-deployment job fails.**

Tag keys and tag values must both be at least 3 characters long.

The interfaces of the app-policy are matched against the interfaces of the
edge-app definition manifest by `intfname`, so `app_eth0`/`app_eth1`/`app_eth2`
must be exactly the names used in `edge_apps.tf`. The ACLs — including the
`10022 -> 22` SSH port map — come from the manifest and do not have to be
repeated in the policy.

## Cloud-init / custom config

An app-policy has nowhere to store per-instance custom config values, so when
Zedcloud renders the cloud-init template for an auto-deployed instance it
substitutes each variable with the `value` from the edge-app *definition*,
falling back to `default` when `value` is empty. `edge_apps.tf` therefore sets
`value` on the definition's variables (`local.UBUNTU_CLOUD_INIT_OVERRIDES`)
rather than on an instance resource.

The practical consequence: every instance auto-deployed from this policy gets
the same user, SSH public key and password.

## The persistent data disk

The second drive of the edge-app manifest has **no `volumelabel`**. In
`1_node_1_vm` that label ties the drive to an explicit
`zedcloud_volume_instance`, but here the app-instance is created by Zedcloud
right after the edge-node, before Terraform could create such a volume — and
Zedcloud fails the deployment if a labelled drive has no matching
volume-instance on the node. Without a label Zedcloud creates an implicit blank
block-storage volume of `maxsize` instead (20GB, expressed in **kilobytes** in
the manifest).

## Lifecycle / caveats

- The auto-created network-instances and edge-app-instance are **not** in the
  Terraform state. `tofu plan` will not show drift if someone edits them in the
  Zedcloud UI, and `tofu destroy` does not delete them directly — they are
  removed by Zedcloud when the edge-node leaves the project or is deleted.
- Editing the app-policy or the network-policy does **not** retroactively update
  already deployed instances; it only affects nodes that join afterwards.
- `vminfo` (e.g. the `cpus = 6` override of `1_node_1_vm`) cannot be set from an
  app-policy — the auto-deployed instance uses the `resources` of the edge-app
  manifest, so 2 vCPUs here.
- The `zedcloud_project` resource now depends on `zedcloud_application`, since
  the app-policy references the edge-app by id and name, and Zedcloud validates
  that the edge-app exists when the policy is created.

## Usage

Same as `1_node_1_vm`:

```
tofu init
tofu apply
tofu output -json AUTO_DEPLOYED_EDGE_APP_INSTANCE
```

`ZEDEDA_CLOUD_URL` / `ZEDEDA_CLOUD_TOKEN` have to be provided (e.g. via
`TF_VAR_…` environment variables) and `edge_node_ssh_pub_key` / `config_suffix`
via a `terraform.tfvars`.
