locals {
  # This is a very convoluted way of taking the same list of Zedcloud custom config
  # variables that were used when creating the edge-app definition and updating some
  # of those variables with specific values for a specific edge-app-instance. This
  # kind of simulates what an user would do in the Zedcontrol WEB UI when creating
  # an edge-app-instance and setting some of the custom config variables.
  APP_INST_CUSTOM_CONF_OVERRIDES = {
    "HELLO_USERNAME" = {
      value = ""
    },
    "HELLO_PASSWORD" = {
      value = ""
    },
  }

  # Create a deep copy of the entire list of custom config variables with the
  # overrides applied.
  APP_INST_CUSTOM_CONF_VARS = [
    for xxx in var.CONTAINER_APP_CUSTOM_CONFIG_VARS : merge(xxx,
      # Only try to merge if there's an override for this variable.
      contains(keys(local.APP_INST_CUSTOM_CONF_OVERRIDES), xxx.name)
      ? local.APP_INST_CUSTOM_CONF_OVERRIDES[xxx.name]
      : {}
    )
  ]

  nodes = {
    "ENODE_TEST" = zedcloud_edgenode.ENODE_TEST
  }
}

resource "zedcloud_network_instance" "NET_INSTANCES_APP_NAT" {
  for_each = local.nodes

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

resource "zedcloud_volume_instance" "APP_PERSIST_STORAGE" {
  for_each = local.nodes

  name  = "app_persist_storage_${each.value.name}_${var.config_suffix}"
  title = "app_persist_storage_${each.value.name}_${var.config_suffix}"

  project_id = zedcloud_project.PROJECT.id
  device_id  = each.value.id

  type       = "VOLUME_INSTANCE_TYPE_BLOCKSTORAGE"
  accessmode = "VOLUME_INSTANCE_ACCESS_MODE_READWRITE"
  size_bytes = 1048576 #### 1GB

  image       = ""
  multiattach = false
  cleartext   = true

  label = zedcloud_application.CONTAINER_APP_DEF.manifest[0].images[1].volumelabel

  #### lifecycle {
  ####  prevent_destroy = true
  #### }
}

resource "zedcloud_application_instance" "APP_INSTANCES_CONTAINERS" {
  for_each = local.nodes

  depends_on = [
    zedcloud_volume_instance.APP_PERSIST_STORAGE
  ]

  name      = "ubuntu_cloud_vm_test_on_${each.value.id}"
  title     = "TF created instance of ${zedcloud_application.CONTAINER_APP_DEF.name} for ${each.value.name}"
  device_id = each.value.id
  app_id    = zedcloud_application.CONTAINER_APP_DEF.id
  app_type  = zedcloud_application.CONTAINER_APP_DEF.manifest[0].app_type

  activate = true

  logs {
    access = true
  }

  vminfo {
    cpus = 1 # zedcloud_application.CONTAINER_APP_DEF.manifest[0].resources[???].value
    mode = zedcloud_application.CONTAINER_APP_DEF.manifest[0].vmmode
    vnc  = false
  }

  interfaces {
    intfname    = zedcloud_application.CONTAINER_APP_DEF.manifest[0].interfaces[0].name
    intforder   = 1
    privateip   = false
    netinstname = zedcloud_network_instance.NET_INSTANCES_APP_NAT[each.key].name
  }

  # The `custom_config` section is identical to what is in the edge-app definition,
  # only that for generating the list of variables we use the per-instance list
  # of variables (`local.APP_INST_CUSTOM_CONF_VARS`) instead of the
  # list which was used in the edge-app definition (`var.CONTAINER_APP_CUSTOM_CONFIG_VARS`).
  custom_config {
    add                  = true
    allow_storage_resize = false
    field_delimiter      = "###"
    name                 = "config01"
    override             = false
    template             = filebase64("${path.module}/edge_app_custom_config.txt")

    variable_groups {
      name     = "Default Group 1"
      required = true

      dynamic "variables" {
        for_each = local.APP_INST_CUSTOM_CONF_VARS
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
}

output "EDGE_APP_INSTANCES" {
  description = "Print edge-app-instances which have been created for every edge-node which joined the project"
  value = {
    for x in zedcloud_application_instance.APP_INSTANCES_CONTAINERS : x.name => {
      id = x.id
    }
  }
}

output "EDGE_APP_LOCALHOST_PORT" {
  description = "Due to the 2 levels of port-forwarding the edge-app-instance container port 8080 is available on localhost on port"
  value       = zedamigo_edge_node.ENODE_TEST_VM.ssh_port + 2
}
