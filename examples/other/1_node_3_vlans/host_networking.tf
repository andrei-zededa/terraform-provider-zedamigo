# Host-side networking for the edge-node's VLANs-only trunk NIC (eth1).
#
# A single TAP interface backs the edge-node's eth1. On the host we create three
# VLAN sub-interfaces (506, 507, 508) on top of that TAP. Each VLAN is then
# enslaved to its own bridge, and each bridge has a distinct IPv4 subnet with a
# DHCPv4 server. This simulates an upstream switch presenting three tagged VLANs
# on the trunk link to the edge-node.
#
# No network namespaces are used: everything lives in the host's default
# namespace, so the bridges/DHCP servers are directly reachable from the host.
#
#   eth1 (edge-node) ── TAP_TRUNK
#                          ├── VLAN 506 ──> BRIDGE_506 (10.99.6.1/24)  + DHCP_506
#                          ├── VLAN 507 ──> BRIDGE_507 (10.99.7.1/24)  + DHCP_507
#                          └── VLAN 508 ──> BRIDGE_508 (10.99.8.1/24)  + DHCP_508

resource "zedamigo_tap" "TAP_TRUNK" {
  name  = "trnk-${var.config_suffix}"
  mtu   = "1500"
  state = "up"
  group = "kvm"
}

############################ VLAN 506 ############################

resource "zedamigo_vlan" "VLAN_506" {
  parent  = zedamigo_tap.TAP_TRUNK.name
  vlan_id = 506
  mtu     = "1500"
  state   = "up"
}

resource "zedamigo_bridge" "BRIDGE_506" {
  name                = "br506-${var.config_suffix}"
  mtu                 = "1500"
  state               = "up"
  ipv4_address        = "10.99.56.1/24"
  enslaved_interfaces = [zedamigo_vlan.VLAN_506.name]
}

resource "zedamigo_dhcp_server" "DHCP_506" {
  interface  = zedamigo_bridge.BRIDGE_506.name
  server_id  = "10.99.56.1"
  nameserver = "9.9.9.9"
  router     = "10.99.56.1"
  netmask    = "255.255.255.0"
  pool {
    start = "10.99.56.70"
    end   = "10.99.56.79"
  }
  lease_time = 86400
}

############################ VLAN 507 ############################

resource "zedamigo_vlan" "VLAN_507" {
  parent  = zedamigo_tap.TAP_TRUNK.name
  vlan_id = 507
  mtu     = "1500"
  state   = "up"
}

resource "zedamigo_bridge" "BRIDGE_507" {
  name                = "br507-${var.config_suffix}"
  mtu                 = "1500"
  state               = "up"
  ipv4_address        = "10.99.57.1/24"
  enslaved_interfaces = [zedamigo_vlan.VLAN_507.name]
}

resource "zedamigo_dhcp_server" "DHCP_507" {
  interface  = zedamigo_bridge.BRIDGE_507.name
  server_id  = "10.99.57.1"
  nameserver = "9.9.9.9"
  router     = "10.99.57.1"
  netmask    = "255.255.255.0"
  pool {
    start = "10.99.57.70"
    end   = "10.99.57.79"
  }
  lease_time = 86400
}

############################ VLAN 508 ############################

resource "zedamigo_vlan" "VLAN_508" {
  parent  = zedamigo_tap.TAP_TRUNK.name
  vlan_id = 508
  mtu     = "1500"
  state   = "up"
}

resource "zedamigo_bridge" "BRIDGE_508" {
  name                = "br508-${var.config_suffix}"
  mtu                 = "1500"
  state               = "up"
  ipv4_address        = "10.99.58.1/24"
  enslaved_interfaces = [zedamigo_vlan.VLAN_508.name]
}

resource "zedamigo_dhcp_server" "DHCP_508" {
  interface  = zedamigo_bridge.BRIDGE_508.name
  server_id  = "10.99.58.1"
  nameserver = "9.9.9.9"
  router     = "10.99.58.1"
  netmask    = "255.255.255.0"
  pool {
    start = "10.99.58.70"
    end   = "10.99.58.79"
  }
  lease_time = 86400
}
