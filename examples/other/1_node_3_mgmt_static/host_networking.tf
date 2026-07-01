# Host-side networking for the edge-node's two statically-addressed management
# NICs (eth1: static IPv4, eth2: static IPv6).
#
# Each NIC is backed on the host by a TAP enslaved to its own bridge. The bridge
# holds the gateway address of the subnet, so the statically-configured node
# interface has something to talk to (ARP / ND / gateway). There is no DHCP
# server: the addressing is fully static and is driven from the controller
# config (see networks.tf / edge_nodes.tf).
#
# No network namespaces are used: everything lives in the host's default
# namespace, so the bridges are directly reachable from the host.
#
#   eth1 (edge-node) ── TAP_1 ──> BRIDGE_1 (10.99.61.1/24)      node uses .10
#   eth2 (edge-node) ── TAP_2 ──> BRIDGE_2 (fd00:99:62::1/64)   node uses ::10

############################ eth1 / BRIDGE_1 ############################

resource "zedamigo_bridge" "BRIDGE_1" {
  name         = "brm1-${var.config_suffix}"
  mtu          = "1500"
  state        = "up"
  ipv4_address = "10.99.61.1/24"
}

resource "zedamigo_tap" "TAP_1" {
  name   = "tapm1-${var.config_suffix}"
  mtu    = "1500"
  state  = "up"
  group  = "kvm"
  master = zedamigo_bridge.BRIDGE_1.name
}

############################ eth2 / BRIDGE_2 ############################

resource "zedamigo_bridge" "BRIDGE_2" {
  name         = "brm2-${var.config_suffix}"
  mtu          = "1500"
  state        = "up"
  ipv6_address = "fd00:99:62::1/64"
}

resource "zedamigo_tap" "TAP_2" {
  name   = "tapm2-${var.config_suffix}"
  mtu    = "1500"
  state  = "up"
  group  = "kvm"
  master = zedamigo_bridge.BRIDGE_2.name
}
