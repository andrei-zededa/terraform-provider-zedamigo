resource "zedcloud_network_instance" "local_nat" {
  for_each = var.nodes

  name      = "ni_local_nat_${each.key}_${var.name_suffix}"
  title     = "Local NAT network instance for edge_node_${each.key}"
  kind      = "NETWORK_INSTANCE_KIND_LOCAL"
  type      = "NETWORK_INSTANCE_DHCP_TYPE_V4"
  device_id = module.edge_node[each.key].id

  port           = "eth0"
  device_default = true

  tags = {
    ni_local_nat = "true"
  }
}

resource "zedcloud_network_instance" "app_shared" {
  for_each = var.nodes

  name      = "ni_app_shared_${each.key}_${var.name_suffix}"
  title     = "App shared network instance for edge_node_${each.key}"
  kind      = "NETWORK_INSTANCE_KIND_SWITCH"
  type      = "NETWORK_INSTANCE_DHCP_TYPE_UNSPECIFIED"
  device_id = module.edge_node[each.key].id

  port = "eth1"

  tags = {}
}
