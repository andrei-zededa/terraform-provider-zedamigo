resource "zedcloud_application_instance" "vm_instance" {
  for_each = var.nodes

  name      = "vm_on_${module.edge_node[each.key].name}"
  title     = "Instance of ${data.zedcloud_application.enterprise.name} on ${module.edge_node[each.key].name}"
  device_id = module.edge_node[each.key].id
  app_id    = data.zedcloud_application.enterprise.id
  app_type  = data.zedcloud_application.enterprise.manifest[0].app_type

  activate = true

  logs {
    access = true
  }

  custom_config {
    add                  = true
    allow_storage_resize = false
    override             = false
  }

  manifest_info {
    transition_action = "INSTANCE_TA_NONE"
  }

  vminfo {
    cpus = 1
    mode = data.zedcloud_application.enterprise.manifest[0].vmmode
    vnc  = false
  }

  drives {
    cleartext = true
    mountpath = "/"
    imagename = data.zedcloud_application.enterprise.manifest[0].images[0].imagename
    maxsize   = "20971520"
    preserve  = false
    readonly  = false
    drvtype   = ""
    target    = ""
  }

  # Here we "know" that the app definition has 2 interfaces. This could be replaced
  # with a dynamic block based on the length of `data.zedcloud_application.enterprise.manifest[0].interfaces`.
  interfaces {
    intfname    = data.zedcloud_application.enterprise.manifest[0].interfaces[0].name
    intforder   = 1
    privateip   = false
    netinstname = zedcloud_network_instance.local_nat[each.key].name
  }

  interfaces {
    intfname    = data.zedcloud_application.enterprise.manifest[0].interfaces[1].name
    intforder   = 2
    privateip   = false
    netinstname = zedcloud_network_instance.app_shared[each.key].name
  }
}
