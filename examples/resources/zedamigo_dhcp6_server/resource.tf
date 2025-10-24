variable "intf_to_run_dhcp6" {
  sensitive = false
  type      = string
  default   = "eth1"
}

resource "zedamigo_dhcp6_server" "test" {
  interface  = var.intf_to_run_dhcp6
  prefix     = "fd00:abcd:1234::/64"
  nameserver = "2606:4700:4700::1111"
  pool_start = "fd00:abcd:1234::100"
  pool_end   = "fd00:abcd:1234::199"
}
