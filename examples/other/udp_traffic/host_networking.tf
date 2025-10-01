resource "zedamigo_bridge" "BRIDGE_101" {
  name         = "br101-${var.config_suffix}"
  mtu          = "1500"
  state        = "up"
  ipv4_address = "10.99.101.1/24"
}

resource "zedamigo_dhcp_server" "DHCP_101" {
  interface  = zedamigo_bridge.BRIDGE_101.name
  server_id  = "10.99.101.1"
  nameserver = "9.9.9.9"
  router     = "10.99.101.1"
  netmask    = "255.255.255.0"
  pool_start = "10.99.101.70"
  pool_end   = "10.99.101.79"
}

resource "zedamigo_tap" "TAP_101" {
  name   = "tap101-${var.config_suffix}"
  mtu    = "1500"
  state  = "up"
  group  = "kvm"
  master = zedamigo_bridge.BRIDGE_101.name
}

resource "zedamigo_bridge" "BRIDGE_102" {
  name         = "br102-${var.config_suffix}"
  mtu          = "1500"
  state        = "up"
  ipv4_address = "10.99.102.1/24"
}

resource "zedamigo_dhcp_server" "DHCP_102" {
  interface  = zedamigo_bridge.BRIDGE_102.name
  server_id  = "10.99.102.1"
  nameserver = "9.9.9.9"
  router     = "10.99.102.1"
  netmask    = "255.255.255.0"
  pool_start = "10.99.102.80"
  pool_end   = "10.99.102.89"
}

resource "zedamigo_tap" "TAP_102" {
  name   = "tap102-${var.config_suffix}"
  mtu    = "1500"
  state  = "up"
  group  = "kvm"
  master = zedamigo_bridge.BRIDGE_102.name
}

resource "zedamigo_bridge" "BRIDGE_103" {
  name         = "br103-${var.config_suffix}"
  mtu          = "1500"
  state        = "up"
  ipv4_address = "10.99.103.1/24"
}

resource "zedamigo_dhcp_server" "DHCP_103" {
  interface  = zedamigo_bridge.BRIDGE_103.name
  server_id  = "10.99.103.1"
  nameserver = "9.9.9.9"
  router     = "10.99.103.1"
  netmask    = "255.255.255.0"
  pool_start = "10.99.103.90"
  pool_end   = "10.99.103.99"
}

resource "zedamigo_tap" "TAP_103" {
  name   = "tap103-${var.config_suffix}"
  mtu    = "1500"
  state  = "up"
  group  = "kvm"
  master = zedamigo_bridge.BRIDGE_103.name
}
