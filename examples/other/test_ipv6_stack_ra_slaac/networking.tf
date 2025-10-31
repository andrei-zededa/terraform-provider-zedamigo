resource "zedamigo_bridge" "BRIDGE_01" {
  name         = "br-ipv6-a"
  mtu          = "1500"
  state        = "up"
  ipv4_address = "203.0.113.129/25"
  ipv6_address = "2001:db8:113::1/64"
}

resource "zedamigo_tap" "TAP_01" {
  name   = "tap-ipv6-a"
  mtu    = "1500"
  state  = "up"
  group  = "kvm"
  master = zedamigo_bridge.BRIDGE_01.name
}

# Basic SLAAC setup.
resource "zedamigo_radv" "slaac" {
  interface         = zedamigo_bridge.BRIDGE_01.name
  prefix            = "2001:db8:113::/64"
  dns_servers       = "2606:4700:4700::1111,2606:4700:4700::1001"
  prefix_autonomous = true  # Allow SLAAC.
  managed_config    = false # Don't require DHCPv6 for addresses.
  other_config      = false # Don't require DHCPv6 for other config.
  min_interval      = 20
  max_interval      = 60
}
