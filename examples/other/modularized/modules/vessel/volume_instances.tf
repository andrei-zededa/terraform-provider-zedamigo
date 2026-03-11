resource "zedcloud_volume_instance" "app_vol_ctree_or_bstor" {
  for_each = local.node_app_drive_pairs

  name  = "${each.value.app_name}_on_${module.edge_node[each.value.node_key].name}_vol_${each.value.drive_index}_ctree_or_bstore"
  title = "${each.value.app_name}_on_${module.edge_node[each.value.node_key].name}_vol_${each.value.drive_index}_ctree_or_bstore"

  device_id = module.edge_node[each.value.node_key].id

  size_bytes  = each.value.image_spec.maxsize
  type        = each.value.is_content_tree ? "VOLUME_INSTANCE_TYPE_CONTENT_TREE" : "VOLUME_INSTANCE_TYPE_BLOCKSTORAGE"
  accessmode  = "VOLUME_INSTANCE_ACCESS_MODE_READWRITE"
  multiattach = false
  cleartext   = false

  label = each.value.is_content_tree ? "" : each.value.image_spec.volumelabel
  image = each.value.is_content_tree ? each.value.drive_image_name : ""

  tags = {
    (each.value.image_spec.volumelabel) = ""
  }
}

# Additional blockstorage volume for each content tree drive.
resource "zedcloud_volume_instance" "app_vol_bstor_for_each_ctree" {
  for_each = {
    for key, pair in local.node_app_drive_pairs : key => pair if pair.is_content_tree
  }

  name  = "${each.value.app_name}_on_${module.edge_node[each.value.node_key].name}_vol_${each.value.drive_index}_bstore"
  title = "${each.value.app_name}_on_${module.edge_node[each.value.node_key].name}_vol_${each.value.drive_index}_bstore"

  device_id = module.edge_node[each.value.node_key].id

  # Link to the content-tree previously created.
  content_tree_id = zedcloud_volume_instance.app_vol_ctree_or_bstor[each.key].id

  size_bytes  = each.value.image_spec.maxsize
  type        = "VOLUME_INSTANCE_TYPE_BLOCKSTORAGE"
  accessmode  = "VOLUME_INSTANCE_ACCESS_MODE_READWRITE"
  multiattach = false
  cleartext   = false

  label = each.value.image_spec.volumelabel

  tags = {
    (each.value.image_spec.volumelabel) = ""
  }
}
