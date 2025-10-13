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

resource "zedamigo_tap" "TAP" {
  for_each = local.nodes

  name   = "tap${index(local.node_keys, each.key)}-${var.config_suffix}"
  mtu    = "1500"
  state  = "up"
  group  = "kvm"
  master = zedamigo_bridge.BRIDGE_101.name
}

resource "zedamigo_local_datastore" "LOCAL_DS" {
  static_dir = "./local_ds/"
  listen     = "${split("/", zedamigo_bridge.BRIDGE_101.ipv4_address)[0]}:8080"
}

resource "zedamigo_tap" "TAP_EDGE_SYNC" {
  name   = "tap333-${var.config_suffix}"
  mtu    = "1500"
  state  = "up"
  group  = "kvm"
  master = zedamigo_bridge.BRIDGE_101.name
}
