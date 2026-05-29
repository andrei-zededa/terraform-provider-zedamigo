resource "zedamigo_bridge" "BRIDGE_A" {
  name         = "brA-${var.config_suffix}"
  mtu          = "1500"
  state        = "up"
  ipv4_address = "10.99.1.1/24"
}

resource "zedamigo_dhcp_server" "DHCP_A" {
  interface  = zedamigo_bridge.BRIDGE_A.name
  server_id  = "10.99.1.1"
  nameserver = "9.9.9.9"
  router     = "10.99.1.1"
  netmask    = "255.255.255.0"
  pool {
    start = "10.99.1.50"
    end   = "10.99.1.59"
  }
  lease_time = 86400
}

resource "zedamigo_tap" "TAP_A_1" {
  name   = "tapA-1-${var.config_suffix}"
  mtu    = "1500"
  state  = "up"
  group  = "kvm"
  master = zedamigo_bridge.BRIDGE_A.name
}

resource "zedamigo_tap" "TAP_A_2" {
  name   = "tapA-2-${var.config_suffix}"
  mtu    = "1500"
  state  = "up"
  group  = "kvm"
  master = zedamigo_bridge.BRIDGE_A.name
}

resource "zedamigo_tap" "TAP_A_3" {
  name   = "tapA-3-${var.config_suffix}"
  mtu    = "1500"
  state  = "up"
  group  = "kvm"
  master = zedamigo_bridge.BRIDGE_A.name
}

resource "zedamigo_bridge" "BRIDGE_B" {
  name         = "brB-${var.config_suffix}"
  mtu          = "1500"
  state        = "up"
  ipv4_address = "10.99.2.1/24"
}

resource "zedamigo_dhcp_server" "DHCP_B" {
  interface  = zedamigo_bridge.BRIDGE_B.name
  server_id  = "10.99.2.1"
  nameserver = "9.9.9.9"
  router     = "10.99.2.1"
  netmask    = "255.255.255.0"
  pool {
    start = "10.99.2.60"
    end   = "10.99.2.69"
  }
  lease_time = 86400
}

resource "zedamigo_tap" "TAP_B_1" {
  name   = "tapB-1-${var.config_suffix}"
  mtu    = "1500"
  state  = "up"
  group  = "kvm"
  master = zedamigo_bridge.BRIDGE_B.name
}

resource "zedamigo_tap" "TAP_B_2" {
  name   = "tapB-2-${var.config_suffix}"
  mtu    = "1500"
  state  = "up"
  group  = "kvm"
  master = zedamigo_bridge.BRIDGE_B.name
}

resource "zedamigo_tap" "TAP_B_3" {
  name   = "tapB-3-${var.config_suffix}"
  mtu    = "1500"
  state  = "up"
  group  = "kvm"
  master = zedamigo_bridge.BRIDGE_B.name
}

resource "zedamigo_bridge" "BRIDGE_C" {
  name  = "brC-${var.config_suffix}"
  mtu   = "1500"
  state = "up"
}

resource "zedamigo_tap" "TAP_C_1" {
  name   = "tapC-1-${var.config_suffix}"
  mtu    = "1500"
  state  = "up"
  group  = "kvm"
  master = zedamigo_bridge.BRIDGE_C.name
}

resource "zedamigo_tap" "TAP_C_2" {
  name   = "tapC-2-${var.config_suffix}"
  mtu    = "1500"
  state  = "up"
  group  = "kvm"
  master = zedamigo_bridge.BRIDGE_C.name
}

resource "zedamigo_tap" "TAP_C_3" {
  name   = "tapC-3-${var.config_suffix}"
  mtu    = "1500"
  state  = "up"
  group  = "kvm"
  master = zedamigo_bridge.BRIDGE_C.name
}
