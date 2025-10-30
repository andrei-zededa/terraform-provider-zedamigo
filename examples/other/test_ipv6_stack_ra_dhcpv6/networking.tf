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

# "Managed" RA config.
resource "zedamigo_radv" "managed" {
  interface         = zedamigo_bridge.BRIDGE_01.name
  prefix            = "2001:db8:113::/64"
  prefix_on_link    = true
  prefix_autonomous = false # Do NOT allow SLAAC.
  managed_config    = true  # Require DHCPv6 for addresses.
  other_config      = true  # Require DHCPv6 for other config.
  min_interval      = 20
  max_interval      = 60
}

# Run a DHCPv6 server on the same interface on which we're sending RAs.
resource "zedamigo_dhcp6_server" "dhcpv6_server_01" {
  interface  = zedamigo_bridge.BRIDGE_01.name
  server_id  = zedamigo_bridge.BRIDGE_01.mac_address
  nameserver = "2606:4700:4700::1111"
  pool {
    start = "2001:db8:113::baad:0"
    end   = "2001:db8:113::baad:ff"
  }
}

