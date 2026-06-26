# Container edge-app-instances on the VLAN trunk.
#
# This deploys 3 instances of the container edge-app (CONTAINER_APP_DEF) on the
# single edge-node. Every instance has its single interface attached to the same
# switch network-instance on the eth1 trunk (NET_INSTANCES_SWITCH_ETH1), but each
# instance pins its interface to a different access VLAN (506, 507, 508).
#
# Because the network-instance is of kind switch with `access_vlan_id` set, the
# edge-node tags egress traffic / expects ingress traffic on the matching VLAN.
# Each VLAN is bridged on the host to a subnet with its own DHCPv4 server (see
# host_networking.tf), so every instance should get a lease from the matching
# bridge:
#   - VLAN 506 -> br506-<suffix>  10.99.6.1/24  DHCP .70-.79
#   - VLAN 507 -> br507-<suffix>  10.99.7.1/24  DHCP .70-.79
#   - VLAN 508 -> br508-<suffix>  10.99.8.1/24  DHCP .70-.79

locals {
  # Map of <instance key> => <access VLAN ID> for the container app-instances.
  CONTAINER_APP_VLANS = {
    "vlan506" = 506
    "vlan507" = 507
    "vlan508" = 508
  }

  # This is a very convoluted way of taking the same list of Zedcloud custom config
  # variables that were used when creating the edge-app definition and updating some
  # of those variables with specific values for a specific edge-app-instance. This
  # kind of simulates what an user would do in the Zedcontrol WEB UI when creating
  # an edge-app-instance and setting some of the custom config variables.
  CONTAINER_APP_INST_CUSTOM_CONF_OVERRIDES = {
    "HELLO_USERNAME" = {
      value = var.HELLO_ZEDCLOUD_APP_USERNAME
    },
    "HELLO_PASSWORD" = {
      value = var.HELLO_ZEDCLOUD_APP_PASSWORD
    },
  }

  # Create a deep copy of the entire list of custom config variables with the
  # overrides applied.
  CONTAINER_APP_INST_CUSTOM_CONF_VARS = [
    for xxx in var.CONTAINER_APP_CUSTOM_CONFIG_VARS : merge(xxx,
      # Only try to merge if there's an override for this variable.
      contains(keys(local.CONTAINER_APP_INST_CUSTOM_CONF_OVERRIDES), xxx.name)
      ? local.CONTAINER_APP_INST_CUSTOM_CONF_OVERRIDES[xxx.name]
      : {}
    )
  ]
}

resource "zedcloud_application_instance" "APP_INSTANCES_CONTAINERS_VLANS" {
  for_each = local.CONTAINER_APP_VLANS

  name      = "${var.DOCKERHUB_IMAGE_NAME}_vlan_${each.value}_${var.config_suffix}"
  title     = "TF created instance of ${zedcloud_application.CONTAINER_APP_DEF.name} on VLAN ${each.value}"
  device_id = zedcloud_edgenode.ENODE_TEST.id
  app_id    = zedcloud_application.CONTAINER_APP_DEF.id
  app_type  = zedcloud_application.CONTAINER_APP_DEF.manifest[0].app_type

  activate = true

  logs {
    access = true
  }

  vminfo {
    cpus = 1 # zedcloud_application.CONTAINER_APP_DEF.manifest[0].resources[???].value
    mode = zedcloud_application.CONTAINER_APP_DEF.manifest[0].vmmode
    vnc  = true
  }

  # Attach the single app interface to the eth1 switch network-instance and pin
  # it to this instance's access VLAN (506 / 507 / 508).
  interfaces {
    intfname       = zedcloud_application.CONTAINER_APP_DEF.manifest[0].interfaces[0].name
    intforder      = 1
    privateip      = false
    netinstname    = zedcloud_network_instance.NET_INSTANCES_SWITCH_ETH1.name
    access_vlan_id = each.value
  }

  # The `custom_config` section is identical to what is in the edge-app definition,
  # only that for generating the list of variables we use the per-instance list
  # of variables (`local.CONTAINER_APP_INST_CUSTOM_CONF_VARS`) instead of the
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
        for_each = local.CONTAINER_APP_INST_CUSTOM_CONF_VARS
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

output "CONTAINER_APP_INSTANCES_VLANS" {
  description = "Container edge-app-instances created on VLANs 506/507/508 of the eth1 switch network-instance"
  value = {
    for k, x in zedcloud_application_instance.APP_INSTANCES_CONTAINERS_VLANS : x.name => {
      id             = x.id
      access_vlan_id = local.CONTAINER_APP_VLANS[k]
    }
  }
}
