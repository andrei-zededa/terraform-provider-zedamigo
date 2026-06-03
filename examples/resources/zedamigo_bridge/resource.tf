resource "zedamigo_bridge" "br1000" {
  name         = "test-br-1000"
  mtu          = "1500"
  state        = "up"
  mac_address  = "02:00:00:72:b3:01"
  ipv4_address = "172.27.199.129/25"
  ipv6_address = "2000:abcd::1/64"

  # Existing interfaces to attach as members of the bridge. Each one is
  # enslaved (`ip link set dev <interface> master <bridge>`) and brought up.
  # The interfaces must already exist in the same network namespace as the
  # bridge.
  enslaved_interfaces = ["eth1", "eth2"]
}
