resource "zedamigo_bridge" "br1000" {
  name         = "test-br-1000"
  mtu          = "1500"
  state        = "up"
  mac_address  = "02:00:00:72:b3:01"
  ipv4_address = "172.27.199.129/25"
  ipv6_address = "2000:abcd::1/64"
}
