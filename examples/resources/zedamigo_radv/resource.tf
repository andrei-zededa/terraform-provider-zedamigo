variable "intf_to_advertise" {
  sensitive = false
  type      = string
  default   = "eth100"
}

# Basic SLAAC setup.
resource "zedamigo_radv" "slaac" {
  interface         = var.intf_to_advertise
  prefix            = "fd00:abcd:1234::/64"
  dns_servers       = "2606:4700:4700::1111,2606:4700:4700::1001"
  prefix_autonomous = true  # Allow SLAAC.
  managed_config    = false # Don't require DHCPv6 for addresses.
  other_config      = false # Don't require DHCPv6 for other config.
  route {
    prefix = "2001:db8::/32"
  }
  route {
    prefix = "2001:db8:1234:5678::/64"
  }
}

# DHCPv6 only (SLAAC disabled).
resource "zedamigo_radv" "dhcpv6_only" {
  interface         = "eth101"
  prefix            = "fd00:1111:2222::/64"
  prefix_autonomous = false # Disable SLAAC.
  managed_config    = true  # Use DHCPv6 for addresses.
  other_config      = true  # Use DHCPv6 for other config.
}
