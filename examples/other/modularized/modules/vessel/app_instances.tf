locals {
  node_app_pairs = merge([
    for node_key, node in var.nodes : {
      for app_name, app_config in node.apps :
      "${node_key}:${app_name}" => {
        node_key        = node_key
        app_name        = app_name
        cloud_init_vars = app_config.cloud_init_vars
        drive_images    = app_config.drive_images
      }
    }
  ]...)

  node_app_drive_pairs = merge([
    for pair_key, pair in local.node_app_pairs : {
      for idx in range(1, length(data.zedcloud_application.enterprise[pair.app_name].manifest[0].images)) :
      "${pair.node_key}:${pair.app_name}:${idx}" => {
        node_key         = pair.node_key
        app_name         = pair.app_name
        drive_index      = idx
        image_spec       = data.zedcloud_application.enterprise[pair.app_name].manifest[0].images[idx]
        is_content_tree  = contains(keys(pair.drive_images), tostring(idx))
        drive_image_name = lookup(pair.drive_images, tostring(idx), null)
      }
    }
  ]...)
}

resource "zedcloud_application_instance" "vm_instance" {
  for_each = local.node_app_pairs

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
            value      = lookup(each.value.cloud_init_vars, variables.value.name, try(variables.value.value, ""))
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

  # Boot drive (index 0, always static)
  drives {
    cleartext = true
    mountpath = "/"
    imagename = data.zedcloud_application.enterprise[each.value.app_name].manifest[0].images[0].imagename
    maxsize   = data.zedcloud_application.enterprise[each.value.app_name].manifest[0].images[0].maxsize
    preserve  = false
    readonly  = false
    drvtype   = ""
    target    = ""
  }

  # Additional drives (index 1+) — dynamically generated from the app manifest
  dynamic "drives" {
    for_each = {
      for idx in range(1, length(data.zedcloud_application.enterprise[each.value.app_name].manifest[0].images)) :
      idx => data.zedcloud_application.enterprise[each.value.app_name].manifest[0].images[idx]
    }
    content {
      cleartext = false
      mountpath = drives.value.mountpath
      # imagename = drives.value.volumelabel
      # This should create the same config while having the app-instance to volume-instance dependency specific
      # (instead of depending on ALL volume-instances since we also remove:
      #     zedcloud_volume_instance.app_vol_ctree_or_bstor,
      #     zedcloud_volume_instance.app_vol_bstor_for_each_ctree
      # from `depends_on`).
      imagename = try(
        zedcloud_volume_instance.app_vol_bstor_for_each_ctree["${each.value.node_key}:${each.value.app_name}:${drives.key}"].label,
        zedcloud_volume_instance.app_vol_ctree_or_bstor["${each.value.node_key}:${each.value.app_name}:${drives.key}"].label
      )

      maxsize  = drives.value.maxsize
      preserve = true
      readonly = false
      drvtype  = drives.value.drvtype
      target   = drives.value.target
    }
  }

  # This mostly handles app definitions with 2 or more interfaces. The first interface defined
  # in the app definition will be connected to the "app traffic" network-instance on the node,
  # while the 2nd and any subsequent interfaces (if they exist) will be connected with the
  # network-instance with tag "local_nat".
  dynamic "interfaces" {
    for_each = data.zedcloud_application.enterprise[each.value.app_name].manifest[0].interfaces
    content {
      intfname    = interfaces.value.name
      intforder   = interfaces.key + 1
      privateip   = false
      netinstname = ""
      # netinsttag  = interfaces.key == 0 ? { ni_local_nat = "true" } : { app_traffic = "app1" }
      # This should create exactly the same config as before will also creating a specific dependency
      # to one network-instance instead of all (since we completly removed the `depends_on` block).
      netinsttag = interfaces.key == 0 ? zedcloud_network_instance.app_shared[each.value.node_key].tags : zedcloud_network_instance.local_nat[each.value.node_key].tags
    }
  }

  lifecycle {
    ignore_changes = [custom_config]
  }
}
