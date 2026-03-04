locals {
  node_app_pairs = merge([
    for node_key, node in var.nodes : {
      for app_name, variable_overrides in node.apps :
      "${node_key}:${app_name}" => {
        node_key           = node_key
        app_name           = app_name
        variable_overrides = variable_overrides
      }
    }
  ]...)
}

resource "zedcloud_application_instance" "vm_instance" {
  for_each = local.node_app_pairs

  # Because the app-instances will match network-instances and volume-instances
  # based on tags we need to specifically set this dependency.
  depends_on = [
    zedcloud_network_instance.local_nat,
    zedcloud_network_instance.app_shared,
    zedcloud_volume_instance.persist_vol
  ]

  name      = "${each.value.app_name}_on_${module.edge_node[each.value.node_key].name}"
  title     = "Instance of ${data.zedcloud_application.enterprise[each.value.app_name].name} on ${module.edge_node[each.value.node_key].name}"
  device_id = module.edge_node[each.value.node_key].id
  app_id    = data.zedcloud_application.enterprise[each.value.app_name].id
  app_type  = data.zedcloud_application.enterprise[each.value.app_name].manifest[0].app_type

  activate = true

  logs {
    access = true
  }

  # Mirror the app definition's custom_config, applying per-instance variable
  # overrides. This is analogous to what an operator does in the ZedControl UI
  # when creating an app instance and setting custom config variables.
  custom_config {
    add                  = true
    allow_storage_resize = false
    override             = false
    field_delimiter      = try(data.zedcloud_application.enterprise[each.value.app_name].manifest[0].configuration[0].custom_config[0].field_delimiter, "")
    name                 = try(data.zedcloud_application.enterprise[each.value.app_name].manifest[0].configuration[0].custom_config[0].name, "")

    dynamic "variable_groups" {
      for_each = try(data.zedcloud_application.enterprise[each.value.app_name].manifest[0].configuration[0].custom_config[0].variable_groups, [])
      content {
        name     = variable_groups.value.name
        required = variable_groups.value.required

        dynamic "variables" {
          for_each = variable_groups.value.variables
          content {
            name       = variables.value.name
            default    = variables.value.default
            required   = variables.value.required
            label      = variables.value.label
            format     = variables.value.format
            encode     = variables.value.encode
            max_length = variables.value.max_length
            value      = lookup(each.value.variable_overrides, variables.value.name, try(variables.value.value, ""))
          }
        }
      }
    }
  }

  manifest_info {
    transition_action = "INSTANCE_TA_NONE"
  }

  vminfo {
    cpus = 1
    mode = data.zedcloud_application.enterprise[each.value.app_name].manifest[0].vmmode
    vnc  = data.zedcloud_application.enterprise[each.value.app_name].manifest[0].enablevnc
  }

  drives {
    cleartext = true
    mountpath = "/"
    imagename = data.zedcloud_application.enterprise[each.value.app_name].manifest[0].images[0].imagename
    maxsize   = "20971520"
    preserve  = false
    readonly  = false
    drvtype   = ""
    target    = ""
  }

  # Persistent volume drive - binds to the volume instance created in volume_instances.tf
  drives {
    cleartext = true
    mountpath = data.zedcloud_application.enterprise[each.value.app_name].manifest[0].images[1].mountpath
    imagename = data.zedcloud_application.enterprise[each.value.app_name].manifest[0].images[1].volumelabel
    maxsize   = "0"
    preserve  = true
    readonly  = false
    drvtype   = data.zedcloud_application.enterprise[each.value.app_name].manifest[0].images[1].drvtype
    target    = data.zedcloud_application.enterprise[each.value.app_name].manifest[0].images[1].target
  }

  # This mostly handles app definitions with 2 or more interfaces. The 2nd and any
  # subsequent interface will be connected with the network-instance with tag "app_traffic",
  dynamic "interfaces" {
    for_each = data.zedcloud_application.enterprise[each.value.app_name].manifest[0].interfaces
    content {
      intfname    = interfaces.value.name
      intforder   = interfaces.key + 1
      privateip   = false
      netinstname = ""
      netinsttag  = interfaces.key == 0 ? { ni_local_nat = "true" } : { app_traffic = "app1" }
    }
  }

  lifecycle {
    ignore_changes = [custom_config]
  }
}
