variable "intf_to_advertise" {
  sensitive = false
  type      = string
  default   = "eth1"
}

# Basic SLAAC setup.
resource "zedamigo_radv" "slaac" {
  interface         = var.intf_to_advertise
  prefix            = "fd00:abcd:1234::/64"
  dns_servers       = "2606:4700:4700::1111,2606:4700:4700::1001"
  prefix_autonomous = true  # Allow SLAAC.
  managed_config    = false # Don't require DHCPv6 for addresses.
  other_config      = false # Don't require DHCPv6 for other config.
}

# DHCPv6 only (SLAAC disabled).
resource "zedamigo_radv" "dhcpv6_only" {
  interface         = "eth2"
  prefix            = "fd00:1111:2222::/64"
  prefix_autonomous = false # Disable SLAAC.
  managed_config    = true  # Use DHCPv6 for addresses.
  other_config      = true  # Use DHCPv6 for other config.
}
