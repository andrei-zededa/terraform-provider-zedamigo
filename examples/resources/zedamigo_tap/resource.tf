resource "zedamigo_bridge" "br1000" {
  name         = "test-br-1000"
  mtu          = "1500"
  state        = "up"
  ipv4_address = "172.27.199.129/25"
}

resource "zedamigo_tap" "tap1000" {
  name   = "test-tap-1000"
  mtu    = "1500"
  state  = "up"
  group  = "kvm"
  master = zedamigo_bridge.br1000.name
}

resource "zedamigo_tap" "tap2000" {
  name         = "test-tap-2000"
  mtu          = "1500"
  state        = "up"
  ipv4_address = "172.27.203.225/27"
}
