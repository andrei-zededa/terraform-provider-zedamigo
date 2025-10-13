resource "zedcloud_network_instance" "NET_INSTANCES_APP_NAT" {
  for_each = zedcloud_edgenode.ENODE_TEST

  name      = "ni_local_nat_${each.value.name}_${var.config_suffix}"
  title     = "TF auto-created instance of ni_local_nat for ${each.value.name}"
  kind      = "NETWORK_INSTANCE_KIND_LOCAL"
  type      = "NETWORK_INSTANCE_DHCP_TYPE_V4"
  device_id = each.value.id

  port           = "uplink"
  device_default = true

  tags = {
    ni_local_nat = "true"
  }
}

resource "zedcloud_application_instance" "APP_INSTANCES_VMS" {
  for_each = zedcloud_edgenode.ENODE_TEST

  name      = "ubuntu_cloud_vm_test_on_${each.value.id}"
  title     = "TF created instance of ${zedcloud_application.UBUNTU_CLOUD_VM_DEF.name} for ${each.value.name}"
  device_id = each.value.id
  app_id    = zedcloud_application.UBUNTU_CLOUD_VM_DEF.id
  app_type  = zedcloud_application.UBUNTU_CLOUD_VM_DEF.manifest[0].app_type

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
    mode = zedcloud_application.UBUNTU_CLOUD_VM_DEF.manifest[0].vmmode
    vnc  = false
  }

  drives {
    cleartext = true
    mountpath = "/"
    imagename = zedcloud_image.ubuntu_24_04_server_cloud.name
    maxsize   = "0"
    preserve  = false
    readonly  = false
    drvtype   = ""
    target    = ""
  }

  interfaces {
    intfname    = zedcloud_application.UBUNTU_CLOUD_VM_DEF.manifest[0].interfaces[0].name
    intforder   = 1
    privateip   = false
    netinstname = zedcloud_network_instance.NET_INSTANCES_APP_NAT[each.key].name
  }
}

output "EDGE_APP_INSTANCES" {
  description = "Print edge-app-instances which have been created for every edge-node which joined the project"
  value = {
    for x in zedcloud_application_instance.APP_INSTANCES_VMS : x.name => {
      id = x.id
    }
  }
}
