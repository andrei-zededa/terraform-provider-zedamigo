module "edge_node" {
  for_each = var.nodes
  source   = "../edge-node"

  name        = "edge_node_${each.key}_${var.name_suffix}"
  model_id    = data.zedcloud_model.enterprise[each.value.model_name].id
  project_id  = module.vessel_project.id
  serialno    = each.value.serialno
  ssh_pub_key = each.value.ssh_pub_key
  tags        = each.value.tags
  interfaces  = local.node_interfaces[each.key]
}
