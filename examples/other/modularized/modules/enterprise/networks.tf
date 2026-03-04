resource "zedcloud_network" "default_network_dhcp_client" {
  name  = "default_network_dhcp_client"
  title = "A default network object as a IPv4 DHCP client"
  kind  = "NETWORK_KIND_V4"

  project_id         = zedcloud_project.this.id
  enterprise_default = true

  ip {
    dhcp = "NETWORK_DHCP_TYPE_CLIENT"
  }

  mtu = 1500
}
