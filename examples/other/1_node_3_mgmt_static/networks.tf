# Network objects attached to the edge-node's three management NICs.
#
#   eth0 -> edge_node_as_dhcp_client : DHCP client (QEMU user-mode net + NAT)
#   eth1 -> mgmt_static_eth1         : static IPv4 10.99.61.10/24, gw 10.99.61.1
#   eth2 -> mgmt_static_eth2         : static IPv6 fd00:99:62::10/64, gw fd00:99:62::1
#
# The per-interface static address (`ipaddr`) is set on the edge-node interface
# itself (see edge_nodes.tf); the network object only describes the subnet,
# gateway and DNS servers shared by that segment.

# eth0: the edge-node acts as a DHCP client, getting its address from the QEMU
# embedded DHCP server (10.0.2.0/24) which also provides NAT to the outside.
resource "zedcloud_network" "edge_node_as_dhcp_client" {
  name  = "edge_node_as_dhcp_client_${var.config_suffix}"
  title = "edge_node_as_dhcp_client"
  kind  = "NETWORK_KIND_V4"

  project_id = zedcloud_project.PROJECT.id

  ip {
    dhcp = "NETWORK_DHCP_TYPE_CLIENT"
  }
  mtu = 1500
}

# eth1: static IPv4 configuration. The subnet/gateway match the host bridge
# BRIDGE_1 (see host_networking.tf). The edge-node interface uses the address
# 10.99.61.10 out of this subnet (see edge_nodes.tf).
resource "zedcloud_network" "mgmt_static_eth1" {
  name  = "mgmt_static_eth1_${var.config_suffix}"
  title = "mgmt_static_eth1"
  kind  = "NETWORK_KIND_V4"

  project_id = zedcloud_project.PROJECT.id

  ip {
    dhcp    = "NETWORK_DHCP_TYPE_STATIC"
    subnet  = "10.99.61.0/24"
    gateway = "10.99.61.1"
    dns     = ["9.9.9.9"]
  }
  mtu = 1500
}

# eth2: static IPv6 configuration. The subnet/gateway match the host bridge
# BRIDGE_2 (see host_networking.tf). The edge-node interface uses the address
# fd00:99:62::10 out of this subnet (see edge_nodes.tf). This mirrors the eth1
# static-IPv4 segment above but uses an IPv6 ULA prefix (kind NETWORK_KIND_V6)
# and an IPv6 DNS server (Quad9).
resource "zedcloud_network" "mgmt_static_eth2" {
  name  = "mgmt_static_eth2_${var.config_suffix}"
  title = "mgmt_static_eth2"
  kind  = "NETWORK_KIND_V6"

  project_id = zedcloud_project.PROJECT.id

  ip {
    dhcp    = "NETWORK_DHCP_TYPE_STATIC"
    subnet  = "fd00:99:62::/64"
    gateway = "fd00:99:62::1"
    dns     = ["2620:fe::fe"]
  }
  mtu = 1500
}
