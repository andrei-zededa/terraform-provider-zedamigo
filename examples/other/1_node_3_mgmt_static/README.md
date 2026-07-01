# Edge-node with 3 management interfaces (DHCP + static IPv4 + static IPv6)

This Terraform configuration creates a single edge-node running
**EVE-OS 16.0.1-lts-kvm-amd64** with three management interfaces and the
host-side networking needed to test static IPv4 and static IPv6 management
configuration. It does **not** deploy any edge-app / edge-app-instance — the
node just boots and onboards.

## Edge-node NICs

All three NICs use `ADAPTER_USAGE_MANAGEMENT`.

- `eth0` — the "default nic0" of the QEMU VM, backed by QEMU user-mode
  networking (NAT + embedded DHCP server on `10.0.2.0/24`). It is a
  `NETWORK_DHCP_TYPE_CLIENT` interface and is the port that actually reaches the
  controller / the internet. zedamigo also uses it to expose SSH/port-forwards
  on `localhost`.
- `eth1` — extra management NIC, **static IPv4** `10.99.61.10/24`
  (gateway `10.99.61.1`). On the host it is backed by a TAP enslaved to
  `BRIDGE_1`.
- `eth2` — extra management NIC, **static IPv6** `fd00:99:62::10/64`
  (gateway `fd00:99:62::1`). On the host it is backed by a TAP enslaved to
  `BRIDGE_2`.

## Zedcloud network objects (`networks.tf`)

One network object per NIC:

```
 eth0 -> edge_node_as_dhcp_client : NETWORK_KIND_V4  NETWORK_DHCP_TYPE_CLIENT
 eth1 -> mgmt_static_eth1         : NETWORK_KIND_V4  NETWORK_DHCP_TYPE_STATIC  10.99.61.0/24    gw .1    dns 9.9.9.9
 eth2 -> mgmt_static_eth2         : NETWORK_KIND_V6  NETWORK_DHCP_TYPE_STATIC  fd00:99:62::/64  gw ::1   dns 2620:fe::fe
```

The network object describes the subnet/gateway/DNS shared by the segment; the
specific per-interface address is set on the edge-node interface via `ipaddr`
(see `edge_nodes.tf`).

## Host networking (`host_networking.tf`)

No network namespaces are used — everything is in the host's default namespace.
Each statically-addressed NIC is backed by a TAP enslaved to its own bridge, and
the bridge holds the gateway address of the subnet. There is no DHCP server: the
addressing is fully static and driven from the controller config.

```
 eth1 (edge-node) ── TAP_1 ──> BRIDGE_1 (10.99.61.1/24)      node uses .10
 eth2 (edge-node) ── TAP_2 ──> BRIDGE_2 (fd00:99:62::1/64)   node uses ::10
```

`eth0` does not need any host networking — it is automatically backed by QEMU
user-mode networking.

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
