resource "zedcloud_volume_instance" "persist_vol" {
  for_each = local.node_app_pairs

  name  = "${each.value.app_name}_on_${module.edge_node[each.value.node_key].name}_vol"
  title = "${each.value.app_name}_on_${module.edge_node[each.value.node_key].name}_vol"

  device_id = module.edge_node[each.value.node_key].id

  size_bytes  = tostring(100 * 1024 * 1024) # 100MB
  type        = "VOLUME_INSTANCE_TYPE_EMPTYDIR"
  accessmode  = "VOLUME_INSTANCE_ACCESS_MODE_READWRITE"
  multiattach = false
  cleartext   = true

  label = data.zedcloud_application.enterprise[each.value.app_name].manifest[0].images[1].volumelabel

  tags = {
    (data.zedcloud_application.enterprise[each.value.app_name].manifest[0].images[1].volumelabel) = ""
  }
}
