variable "intf_to_run_dhcp" {
  sensitive = false
  type      = string
  default   = "eth1"
}

resource "zedamigo_dhcp_server" "test" {
  interface  = var.intf_to_run_dhcp
  server_id  = "172.27.244.254"
  nameserver = "9.9.9.9"
  router     = "172.27.244.254"
  netmask    = "255.255.255.0"
  pool {
    start = "172.27.244.100"
    end   = "172.27.244.199"
  }
  static_route {
    to  = "10.10.10.0/24"
    via = "172.27.244.254"
  }
  static_route {
    to  = "11.11.11.0/24"
    via = "172.27.244.254"
  }
  lease_time = 3600 # Optional: lease time in seconds (default: 3600)
}
