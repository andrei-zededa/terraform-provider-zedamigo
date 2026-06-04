resource "zedcloud_volume_instance" "APP_PERSIST_STORAGE" {
  depends_on = [time_sleep.WAIT_AFTER_CLUSTER]

  name  = "app_persist_storage_${var.config_suffix}"
  title = "app_persist_storage_${var.config_suffix}"

  project_id = zedcloud_project.PROJECT.id

  # cluster_id = zedcloud_edgenode_cluster.TEST_CLUSTER.id
  edge_node_cluster {
    id = zedcloud_edgenode_cluster.TEST_CLUSTER.id
  }

  type       = "VOLUME_INSTANCE_TYPE_BLOCKSTORAGE"
  accessmode = "VOLUME_INSTANCE_ACCESS_MODE_READWRITE"
  size_bytes = 1048576 #### 1GB

  image       = ""
  multiattach = false
  cleartext   = true

  label = zedcloud_application.CONTAINER_APP_DEF_WITH_VOL.manifest[0].images[1].volumelabel
}

resource "time_sleep" "WAIT_AFTER_VOLUME" {
  depends_on = [
  zedcloud_volume_instance.APP_PERSIST_STORAGE]

  create_duration = "300s"
}

resource "zedcloud_application_instance" "APP_INSTANCES_CONTAINERS_WITH_VOL" {
  depends_on = [
    time_sleep.WAIT_AFTER_CLUSTER,
    zedcloud_volume_instance.APP_PERSIST_STORAGE,
    time_sleep.WAIT_AFTER_VOLUME,
  ]

  name     = "hello_container_vol_${var.config_suffix}"
  title    = "TF created instance of ${zedcloud_application.CONTAINER_APP_DEF_WITH_VOL.name}"
  app_id   = zedcloud_application.CONTAINER_APP_DEF_WITH_VOL.id
  app_type = zedcloud_application.CONTAINER_APP_DEF_WITH_VOL.manifest[0].app_type

  # cluster_id = zedcloud_edgenode_cluster.TEST_CLUSTER.id
  edge_node_cluster {
    id = zedcloud_edgenode_cluster.TEST_CLUSTER.id
  }

  activate = true

  logs {
    access = true
  }

  vminfo {
    cpus = 1 # zedcloud_application.CONTAINER_APP_DEF_WITH_VOL.manifest[0].resources[???].value
    mode = zedcloud_application.CONTAINER_APP_DEF_WITH_VOL.manifest[0].vmmode
    vnc  = false
  }

  interfaces {
    intfname    = zedcloud_application.CONTAINER_APP_DEF_WITH_VOL.manifest[0].interfaces[0].name
    intforder   = 1
    privateip   = false
    netinstname = zedcloud_network_instance.NET_INSTANCES_APP_NAT.name
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
    template             = filebase64("${path.module}/edge_app_container_custom_config.txt")

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
