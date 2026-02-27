module "edge_node" {
  for_each = var.nodes
  source   = "../edge-node"

  name        = "edge_node_${each.key}_${var.name_suffix}"
  model_id    = data.zedcloud_model.enterprise.id
  project_id  = data.zedcloud_project.enterprise.id
  serialno    = each.value.serialno
  ssh_pub_key = each.value.ssh_pub_key
  tags        = each.value.tags
  interfaces  = each.value.interfaces
}
