locals {
  nodes = {
    "ENODE_TEST_AAAA" = zedcloud_edgenode.ENODE_TEST_AAAA
  }

  # This is a very convoluted way of taking the same list of Zedcloud custom config
  # variables that were used when creating the edge-app definition and updating some
  # of those variables with specific values for a specific edge-app-instance. This
  # kind of simulates what an user would do in the Zedcontrol WEB UI when creating
  # an edge-app-instance and setting some of the custom config variables.
  UBUNTU_CLOUD_INIT_OVERRIDES = {
    "USERNAME" = {
      value = "labuser"
    },
    "SSH_PUB_KEY" = {
      value = var.edge_node_ssh_pub_key
    },
  }

  # Create a deep copy of the entire list of custom config variables with the
  # overrides applied.
  APP_INSTANCE_UBUNTU_CLOUD_INIT_VARS = [
    for xxx in var.UBUNTU_CLOUD_INIT_VARS : merge(xxx,
      # Only try to merge if there's an override for this variable.
      contains(keys(local.UBUNTU_CLOUD_INIT_OVERRIDES), xxx.name)
      ? local.UBUNTU_CLOUD_INIT_OVERRIDES[xxx.name]
      : {}
    )
  ]
}

resource "zedcloud_network_instance" "NET_INSTANCES_LOCAL_APP_NAT" {
  for_each = local.nodes

  name      = "ni_local_nat_uplink_${each.value.name}_${var.config_suffix}"
  title     = "TF created instance of ni_local_nat (port = uplink) for ${each.value.name}"
  kind      = "NETWORK_INSTANCE_KIND_LOCAL"
  type      = "NETWORK_INSTANCE_DHCP_TYPE_V4"
  device_id = each.value.id

  port           = "uplink"
  device_default = true

  tags = {
    ni_local_nat = "true"
  }
}

resource "zedcloud_network_instance" "NET_INSTANCES_SWITCH_3" {
  for_each = local.nodes

  name      = "ni_switch_port3_${each.value.name}_${var.config_suffix}"
  title     = "TF created instance switch (port = port3) for ${each.value.name}"
  kind      = "NETWORK_INSTANCE_KIND_SWITCH"
  type      = "NETWORK_INSTANCE_DHCP_TYPE_UNSPECIFIED"
  device_id = each.value.id

  port = "port3"
}

resource "zedcloud_network_instance" "NET_INSTANCES_SWITCH_4" {
  for_each = local.nodes

  name      = "ni_switch_port4_${each.value.name}_${var.config_suffix}"
  title     = "TF created instance switch (port = port4) for ${each.value.name}"
  kind      = "NETWORK_INSTANCE_KIND_SWITCH"
  type      = "NETWORK_INSTANCE_DHCP_TYPE_UNSPECIFIED"
  device_id = each.value.id

  port = "port4"
}

resource "zedcloud_network_instance" "NET_INSTANCES_SWITCH_5" {
  for_each = local.nodes

  name      = "ni_switch_port5_${each.value.name}_${var.config_suffix}"
  title     = "TF created instance switch (port = port5) for ${each.value.name}"
  kind      = "NETWORK_INSTANCE_KIND_SWITCH"
  type      = "NETWORK_INSTANCE_DHCP_TYPE_UNSPECIFIED"
  device_id = each.value.id

  port = "port5"
}

resource "zedcloud_network_instance" "NET_INSTANCES_SWITCH_6" {
  for_each = local.nodes

  name      = "ni_switch_port6_${each.value.name}_${var.config_suffix}"
  title     = "TF created instance switch (port = port6) for ${each.value.name}"
  kind      = "NETWORK_INSTANCE_KIND_SWITCH"
  type      = "NETWORK_INSTANCE_DHCP_TYPE_UNSPECIFIED"
  device_id = each.value.id

  port = "port6"
}

resource "zedcloud_network_instance" "NET_INSTANCES_SWITCH_7" {
  for_each = local.nodes

  name      = "ni_switch_port7_${each.value.name}_${var.config_suffix}"
  title     = "TF created instance switch (port = port7) for ${each.value.name}"
  kind      = "NETWORK_INSTANCE_KIND_SWITCH"
  type      = "NETWORK_INSTANCE_DHCP_TYPE_UNSPECIFIED"
  device_id = each.value.id

  port = "port7"
}

resource "zedcloud_network_instance" "NET_INSTANCES_SWITCH_8" {
  for_each = local.nodes

  name      = "ni_switch_port8_${each.value.name}_${var.config_suffix}"
  title     = "TF created instance switch (port = port8) for ${each.value.name}"
  kind      = "NETWORK_INSTANCE_KIND_SWITCH"
  type      = "NETWORK_INSTANCE_DHCP_TYPE_UNSPECIFIED"
  device_id = each.value.id

  port = "port8"
}

resource "zedcloud_application_instance" "APP_INSTANCES_VMS" {
  for_each = local.nodes

  name      = "ubuntu_test_on_${each.value.id}"
  title     = "TF created instance of ${zedcloud_application.UBUNTU_VM_DEF.name} for ${each.value.name}"
  device_id = each.value.id
  app_id    = zedcloud_application.UBUNTU_VM_DEF.id
  app_type  = zedcloud_application.UBUNTU_VM_DEF.manifest[0].app_type

  activate = true

  logs {
    access = true
  }

  # The `custom_config` section is identical to what is in the edge-app definition,
  # only that for generating the list of variables we use the per-instance list
  # of variables (`local.APP_INSTANCE_UBUNTU_CLOUD_INIT_VARS`) instead of the
  # list which was used in the edge-app definition (`var.UBUNTU_CLOUD_INIT_VARS`).
  custom_config {
    add                  = true
    allow_storage_resize = false
    field_delimiter      = "####"
    name                 = "config01"
    override             = false
    template             = filebase64("${path.module}/ubuntu_cloud_init.txt")

    variable_groups {
      name     = "Default Group 1"
      required = true

      dynamic "variables" {
        for_each = local.APP_INSTANCE_UBUNTU_CLOUD_INIT_VARS
        content {
          name       = variables.value.name
          default    = variables.value.default
          required   = variables.value.required
          label      = variables.value.label
          format     = variables.value.format
          encode     = variables.value.encode
          max_length = variables.value.max_length
          value      = variables.value.value
        }
      }
    }
  }

  manifest_info {
    transition_action = "INSTANCE_TA_NONE"
  }

  vminfo {
    cpus = 1
    mode = zedcloud_application.UBUNTU_VM_DEF.manifest[0].vmmode
    vnc  = true
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
    intfname    = zedcloud_application.UBUNTU_VM_DEF.manifest[0].interfaces[0].name
    intforder   = 1
    privateip   = false
    netinstname = zedcloud_network_instance.NET_INSTANCES_LOCAL_APP_NAT[each.key].name
  }
  interfaces {
    intfname    = zedcloud_application.UBUNTU_VM_DEF.manifest[0].interfaces[1].name
    intforder   = 2
    privateip   = false
    netinstname = zedcloud_network_instance.NET_INSTANCES_SWITCH_3[each.key].name
  }
  interfaces {
    intfname    = zedcloud_application.UBUNTU_VM_DEF.manifest[0].interfaces[2].name
    intforder   = 3
    privateip   = false
    netinstname = zedcloud_network_instance.NET_INSTANCES_SWITCH_4[each.key].name
  }
  interfaces {
    intfname    = zedcloud_application.UBUNTU_VM_DEF.manifest[0].interfaces[3].name
    intforder   = 4
    privateip   = false
    netinstname = zedcloud_network_instance.NET_INSTANCES_SWITCH_5[each.key].name
  }
  interfaces {
    intfname    = zedcloud_application.UBUNTU_VM_DEF.manifest[0].interfaces[4].name
    intforder   = 5
    privateip   = false
    netinstname = zedcloud_network_instance.NET_INSTANCES_SWITCH_6[each.key].name
  }
  interfaces {
    intfname    = zedcloud_application.UBUNTU_VM_DEF.manifest[0].interfaces[5].name
    intforder   = 6
    privateip   = false
    netinstname = zedcloud_network_instance.NET_INSTANCES_SWITCH_7[each.key].name
  }
  interfaces {
    intfname    = zedcloud_application.UBUNTU_VM_DEF.manifest[0].interfaces[6].name
    intforder   = 7
    privateip   = false
    netinstname = zedcloud_network_instance.NET_INSTANCES_SWITCH_8[each.key].name
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
