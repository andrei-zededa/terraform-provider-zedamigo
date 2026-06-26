# Edge-node with a VLANs-only trunk NIC

This Terraform configuration creates a single edge-node with two NICs and the
host-side networking needed to test VLAN sub-interfaces on a trunk port. It also
creates a VM and a container edge-app *definition*, but **does not** deploy any
edge-app-instance — that part is meant to be done manually.

## Edge-node NICs

- `eth0` — `ADAPTER_USAGE_MANAGEMENT`. This is the "default nic0" of the QEMU VM,
  backed by QEMU user-mode networking (NAT + embedded DHCP server on
  `10.0.2.0/24`). It gives the node management/uplink connectivity and is also
  used by zedamigo to expose SSH/port-forwards on `localhost`.
- `eth1` — `ADAPTER_USAGE_VLANS_ONLY`. A trunk port that carries no untagged
  network of its own; it only carries VLAN sub-interfaces. On the host it is
  backed by a single TAP (`taptrunk-<suffix>`).

## Host networking

No network namespaces are used — everything is in the host's default namespace.
On the TAP backing `eth1` three VLANs are created, each enslaved to its own
bridge, and each bridge has a distinct IPv4 subnet with a DHCPv4 server:

```
 eth1 (edge-node) ── taptrunk-<suffix>
                        ├── VLAN 506 ──> br506-<suffix>  10.99.6.1/24  DHCP .70-.79
                        ├── VLAN 507 ──> br507-<suffix>  10.99.7.1/24  DHCP .70-.79
                        └── VLAN 508 ──> br508-<suffix>  10.99.8.1/24  DHCP .70-.79
```

This simulates an upstream switch presenting three tagged VLANs (506/507/508) on
the trunk link towards the edge-node.

## Edge-app definitions

- A VM edge-app (`UBUNTU_VM_DEF`, Ubuntu 24.04 cloud image) — see `edge_app_vm.tf`.
  No instance is created for this one; deploy it manually if you want to test
  VLANs with a VM instead of a container.
- A container edge-app (`CONTAINER_APP_DEF`, hello-zedcloud) — see
  `edge_app_container.tf`.

## Container edge-app-instances on the VLAN trunk

`edge_app_instances_containers.tf` deploys **3 instances** of the container
edge-app on the single edge-node. There is a single switch network-instance on
the `eth1` trunk (`NET_INSTANCES_SWITCH_ETH1`, see `network_instances.tf`), and
all 3 instances attach their interface to it — but each instance pins its
interface to a different access VLAN via `access_vlan_id`:

```
 eth1 switch network-instance (ni_switch_all_vlans-<suffix>)
   ├── container instance #1 ── access_vlan_id 506 ──> br506-<suffix>  10.99.6.1/24  DHCP .70-.79
   ├── container instance #2 ── access_vlan_id 507 ──> br507-<suffix>  10.99.7.1/24  DHCP .70-.79
   └── container instance #3 ── access_vlan_id 508 ──> br508-<suffix>  10.99.8.1/24  DHCP .70-.79
```

Each instance interface placed on a given VLAN should then receive a DHCPv4
lease from the matching host bridge subnet. The created instances (with their
ids and VLAN IDs) are printed via the `CONTAINER_APP_INSTANCES_VLANS` output.

## Usage

```
export TF_VAR_ZEDEDA_CLOUD_URL="https://zedcontrol.example.zededa.net"
export TF_VAR_ZEDEDA_CLOUD_TOKEN="..."

tofu init
tofu apply
```

The QEMU VM listens on a random `localhost` port for SSH access to EVE-OS; find
it with `tofu state show zedamigo_edge_node.ENODE_TEST_VM` and look at the
`ssh_port` value.
