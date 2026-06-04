locals {
  # This is a very convoluted way of taking the same list of Zedcloud custom config
  # variables that were used when creating the edge-app definition and updating some
  # of those variables with specific values for a specific edge-app-instance. This
  # kind of simulates what an user would do in the Zedcontrol WEB UI when creating
  # an edge-app-instance and setting some of the custom config variables.
  APP_INST_CUSTOM_CONF_OVERRIDES = {
    "HELLO_USERNAME" = {
      value = var.HELLO_ZEDCLOUD_APP_USERNAME
    },
    "HELLO_PASSWORD" = {
      value = var.HELLO_ZEDCLOUD_APP_PASSWORD
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
}

# resource "zedcloud_application_instance" "APP_INSTANCES_CONTAINERS" {
#   depends_on = [time_sleep.WAIT_AFTER_CLUSTER]
# 
#   name      = "hello_container_${var.config_suffix}"
#   title     = "TF created instance of ${zedcloud_application.CONTAINER_APP_DEF.name}"
#   app_id    = zedcloud_application.CONTAINER_APP_DEF.id
#   app_type  = zedcloud_application.CONTAINER_APP_DEF.manifest[0].app_type
# 
#   # cluster_id = zedcloud_edgenode_cluster.TEST_CLUSTER.id
#   edge_node_cluster {
#     id = zedcloud_edgenode_cluster.TEST_CLUSTER.id
#   }
# 
#   activate = true
# 
#   logs {
#     access = true
#   }
# 
#   vminfo {
#     cpus = 1 # zedcloud_application.CONTAINER_APP_DEF.manifest[0].resources[???].value
#     mode = zedcloud_application.CONTAINER_APP_DEF.manifest[0].vmmode
#     vnc  = false
#   }
# 
#   interfaces {
#     intfname    = zedcloud_application.CONTAINER_APP_DEF.manifest[0].interfaces[0].name
#     intforder   = 1
#     privateip   = false
#     netinstname = zedcloud_network_instance.NET_INSTANCES_APP_NAT.name
#   }
# 
#   # The `custom_config` section is identical to what is in the edge-app definition,
#   # only that for generating the list of variables we use the per-instance list
#   # of variables (`local.APP_INST_CUSTOM_CONF_VARS`) instead of the
#   # list which was used in the edge-app definition (`var.CONTAINER_APP_CUSTOM_CONFIG_VARS`).
#   custom_config {
#     add                  = true
#     allow_storage_resize = false
#     field_delimiter      = "###"
#     name                 = "config01"
#     override             = false
#     template             = filebase64("${path.module}/edge_app_container_custom_config.txt")
# 
#     variable_groups {
#       name     = "Default Group 1"
#       required = true
# 
#       dynamic "variables" {
#         for_each = local.APP_INST_CUSTOM_CONF_VARS
#         content {
#           name       = variables.value.name
#           default    = variables.value.default
#           required   = variables.value.required
#           label      = variables.value.label
#           format     = variables.value.format
#           encode     = variables.value.encode
#           max_length = variables.value.max_length
#           value      = variables.value.value
#         }
#       }
#     }
#   }
# }
