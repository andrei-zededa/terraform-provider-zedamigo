resource "zedamigo_bridge" "BRIDGE_01" {
  name         = "k3s-bridge01"
  mtu          = "1500"
  state        = "up"
  ipv4_address = "172.27.213.129/25"
}

resource "zedamigo_tap" "TAP_INTF_CP_01" {
  name   = "k3s-master01"
  mtu    = "1500"
  state  = "up"
  group  = "kvm"
  master = zedamigo_bridge.BRIDGE_01.name
}

resource "zedamigo_tap" "TAP_INTFS" {
  for_each = local.nodes

  name   = "k3s-${each.key}"
  mtu    = "1500"
  state  = "up"
  group  = "kvm"
  master = zedamigo_bridge.BRIDGE_01.name
}
