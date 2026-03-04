resource "zedcloud_network_instance" "local_nat" {
  for_each = var.nodes

  name      = "ni_local_nat_${each.key}"
  title     = "Local NAT network instance for edge_node_${each.key}"
  kind      = "NETWORK_INSTANCE_KIND_LOCAL"
  type      = "NETWORK_INSTANCE_DHCP_TYPE_V4"
  device_id = module.edge_node[each.key].id

  port           = "eth0" # Matches on edge-node adapter name which must be equal to model logical label.
  device_default = true

  tags = {
    ni_local_nat = "true" # This will be used by the app-instance interface.
  }
}

resource "zedcloud_network_instance" "app_shared" {
  for_each = var.nodes

  name      = "ni_app_shared_${each.key}"
  title     = "App shared network instance for edge_node_${each.key}"
  kind      = "NETWORK_INSTANCE_KIND_SWITCH"
  type      = "NETWORK_INSTANCE_DHCP_TYPE_UNSPECIFIED"
  device_id = module.edge_node[each.key].id

  port = "eth1" # Matches on edge-node adapter name which must be equal to model logical label.

  tags = {
    app_traffic = "app1" # This will be used by the app-instance interface.
  }
}
