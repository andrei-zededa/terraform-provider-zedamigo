output "edge_nodes" {
  description = "Map of node keys to edge node details"
  value = {
    for key, node in module.edge_node : key => {
      id       = node.id
      name     = node.name
      serialno = node.serialno
    }
  }
}

output "network_instances" {
  description = "Map of network instance details"
  value = {
    local_nat = {
      for key, ni in zedcloud_network_instance.local_nat : key => {
        id   = ni.id
        name = ni.name
      }
    }
    app_shared = {
      for key, ni in zedcloud_network_instance.app_shared : key => {
        id   = ni.id
        name = ni.name
      }
    }
  }
}

output "app_instances" {
  description = "Map of node_key:app_name composite keys to app instance details"
  value = {
    for key, inst in zedcloud_application_instance.vm_instance : key => {
      id   = inst.id
      name = inst.name
    }
  }
}

output "volume_instances" {
  description = "Map of node_key:app_name:drive_index composite keys to volume instance details"
  value = {
    for key, vol in zedcloud_volume_instance.app_vol_ctree_or_bstor : key => {
      id    = vol.id
      name  = vol.name
      label = vol.label
    }
  }
}

output "content_tree_blockstorage_instances" {
  description = "Map of node_key:app_name:drive_index composite keys to companion blockstorage volume instances for content tree drives"
  value = {
    for key, vol in zedcloud_volume_instance.app_vol_bstor_for_each_ctree : key => {
      id    = vol.id
      name  = vol.name
      label = vol.label
    }
  }
}
