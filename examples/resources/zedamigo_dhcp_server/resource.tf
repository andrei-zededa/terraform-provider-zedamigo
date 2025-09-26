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
  pool_start = "172.27.244.100"
  pool_end   = "172.27.244.199"
}
