resource "zedcloud_network" "default_network_dhcp_client" {
  name  = "default_network_dhcp_client${local.us_name_suffix}"
  title = "A default network object as a IPv4 DHCP client"
  kind  = "NETWORK_KIND_V4"

  project_id = zedcloud_project.this.id

  ip {
    dhcp = "NETWORK_DHCP_TYPE_CLIENT"
  }
  mtu = 1500
}
