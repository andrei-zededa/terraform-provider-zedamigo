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
  description = "Map of app instance keys to details"
  value = {
    for key, inst in zedcloud_application_instance.vm_instance : key => {
      id   = inst.id
      name = inst.name
    }
  }
}
