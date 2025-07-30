resource "zedamigo_bridge" "br1000" {
  name         = "test-br-1000"
  mtu          = "1500"
  state        = "up"
  ipv4_address = "172.27.199.129/25"
}
