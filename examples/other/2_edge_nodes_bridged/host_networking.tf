resource "zedamigo_netns" "TEST_NS_A" {
  name = "TEST_NS_A"
}

resource "zedamigo_bridge" "BRIDGE_A" {
  name         = "br1-${var.config_suffix}"
  mtu          = "1500"
  state        = "up"
  ipv4_address = "10.99.1.1/24"
  netns        = zedamigo_netns.TEST_NS_A.name
}

resource "zedamigo_dhcp_server" "DHCP_A" {
  interface  = zedamigo_bridge.BRIDGE_A.name
  server_id  = "10.99.1.1"
  nameserver = "9.9.9.9"
  router     = "10.99.1.1"
  netmask    = "255.255.255.0"
  pool {
    start = "10.99.1.70"
    end   = "10.99.1.79"
  }
  lease_time = 86400
  netns      = zedamigo_netns.TEST_NS_A.name
}

resource "zedamigo_tap" "TAP_A_AAAA" {
  name   = "tapA-AA-${var.config_suffix}"
  mtu    = "1500"
  state  = "up"
  group  = "kvm"
  master = zedamigo_bridge.BRIDGE_A.name
  netns  = zedamigo_netns.TEST_NS_A.name
}

#? resource "zedamigo_tap" "TAP_A_BBBB" {
#?   name   = "tapA-BB-${var.config_suffix}"
#?   mtu    = "1500"
#?   state  = "up"
#?   group  = "kvm"
#?   master = zedamigo_bridge.BRIDGE_A.name
#?   netns  = zedamigo_netns.TEST_NS_A.name
#? }

resource "zedamigo_netns" "TEST_NS_B" {
  name = "TEST_NS_B"
}

resource "zedamigo_bridge" "BRIDGE_B" {
  name         = "br2-${var.config_suffix}"
  mtu          = "1500"
  state        = "up"
  ipv4_address = "10.99.2.1/24"
  netns        = zedamigo_netns.TEST_NS_B.name
}

resource "zedamigo_dhcp_server" "DHCP_B" {
  interface  = zedamigo_bridge.BRIDGE_B.name
  server_id  = "10.99.2.1"
  nameserver = "9.9.9.9"
  router     = "10.99.2.1"
  netmask    = "255.255.255.0"
  pool {
    start = "10.99.2.70"
    end   = "10.99.2.79"
  }
  lease_time = 86400
  netns      = zedamigo_netns.TEST_NS_B.name
}

resource "zedamigo_tap" "TAP_B_AAAA" {
  name   = "tapB-AA-${var.config_suffix}"
  mtu    = "1500"
  state  = "up"
  group  = "kvm"
  master = zedamigo_bridge.BRIDGE_B.name
  netns  = zedamigo_netns.TEST_NS_B.name
}

#? resource "zedamigo_tap" "TAP_B_BBBB" {
#?   name   = "tapB-BB-${var.config_suffix}"
#?   mtu    = "1500"
#?   state  = "up"
#?   group  = "kvm"
#?   master = zedamigo_bridge.BRIDGE_B.name
#?   netns  = zedamigo_netns.TEST_NS_B.name
#? }
