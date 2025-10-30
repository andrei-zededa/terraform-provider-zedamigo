variable "intf_to_run_dhcp6" {
  sensitive = false
  type      = string
  default   = "eth1"
}

resource "zedamigo_dhcp6_server" "test" {
  interface  = var.intf_to_run_dhcp6
  server_id  = "aa:bb:cc:dd:ee:ff"
  prefix     = "fd00:abcd:1234::/64"
  nameserver = "2606:4700:4700::1111"
  pool {
    start = "fd00:abcd:1234::100"
    end   = "fd00:abcd:1234::199"
  }
  lease_time = 3600 # Optional: lease time in seconds (default: 3600)
}
