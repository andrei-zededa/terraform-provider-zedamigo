resource "zedcloud_network_instance" "NET_INSTANCES_SWITCH_ETH1" {
  name      = "ni_switch_all_vlans_${var.config_suffix}"
  title     = "TF auto-created instance switch (port = eth1)"
  kind      = "NETWORK_INSTANCE_KIND_SWITCH"
  type      = "NETWORK_INSTANCE_DHCP_TYPE_UNSPECIFIED"
  device_id = zedcloud_edgenode.ENODE_TEST.id

  port = "eth1"
}
