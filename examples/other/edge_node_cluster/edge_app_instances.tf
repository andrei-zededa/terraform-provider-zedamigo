resource "random_password" "vm_password" {
  length  = 10
  special = false
}

resource "zedcloud_network_instance" "NET_INSTANCES_APP_NAT" {
  name      = "ni_local_nat_${var.config_suffix}"
  title     = "TF auto-created instance of ni_local_nat"
  kind      = "NETWORK_INSTANCE_KIND_LOCAL"
  type      = "NETWORK_INSTANCE_DHCP_TYPE_V4"
  cluster_id = zedcloud_edgenode_cluster.TEST_CLUSTER.id
  edge_node_cluster {
    id = zedcloud_edgenode_cluster.TEST_CLUSTER.id
  }

  port           = "uplink"
  device_default = true

  tags = {
    ni_local_nat = "true"
  }
}

resource "zedcloud_network_instance" "NET_INSTANCES_SWITCH_FIRST" {
  name      = "ni_switch_first_${var.config_suffix}"
  title     = "TF auto-created instance of first switch"
  kind      = "NETWORK_INSTANCE_KIND_SWITCH"
  type      = "NETWORK_INSTANCE_DHCP_TYPE_UNSPECIFIED"
  cluster_id = zedcloud_edgenode_cluster.TEST_CLUSTER.id
  edge_node_cluster {
    id = zedcloud_edgenode_cluster.TEST_CLUSTER.id
  }

  port = "eth2"
}

resource "zedcloud_network_instance" "NET_INSTANCES_SWITCH_2ND" {
  name      = "ni_switch_2nd_${var.config_suffix}"
  title     = "TF auto-created instance of 2nd switch"
  kind      = "NETWORK_INSTANCE_KIND_SWITCH"
  type      = "NETWORK_INSTANCE_DHCP_TYPE_UNSPECIFIED"
  cluster_id = zedcloud_edgenode_cluster.TEST_CLUSTER.id
  edge_node_cluster {
    id = zedcloud_edgenode_cluster.TEST_CLUSTER.id
  }

  port = "eth3"
}

locals {
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
    "PASSWORD" = {
      value = random_password.vm_password.result
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

resource "zedcloud_application_instance" "APP_INSTANCES_VMS" {
  name      = "ubuntu_test_${var.config_suffix}"
  title     = "TF created instance of ${zedcloud_application.UBUNTU_VM_DEF.name}"
  cluster_id = zedcloud_edgenode_cluster.TEST_CLUSTER.id
  edge_node_cluster {
    id = zedcloud_edgenode_cluster.TEST_CLUSTER.id
  }
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
    cpus = 6
    mode = zedcloud_application.UBUNTU_VM_DEF.manifest[0].vmmode
    vnc  = true
  }

  drives {
    cleartext = true
    mountpath = "/"
    imagename = zedcloud_application.UBUNTU_VM_DEF.manifest[0].images[0].imagename
    maxsize   = zedcloud_application.UBUNTU_VM_DEF.manifest[0].images[0].maxsize
    preserve  = false
    readonly  = false
    drvtype   = ""
    target    = ""
  }

  interfaces {
    intfname    = zedcloud_application.UBUNTU_VM_DEF.manifest[0].interfaces[0].name
    intforder   = 1
    privateip   = false
    netinstname = zedcloud_network_instance.NET_INSTANCES_APP_NAT.name
  }

  interfaces {
    intfname    = zedcloud_application.UBUNTU_VM_DEF.manifest[0].interfaces[1].name
    intforder   = 2
    privateip   = false
    netinstname = zedcloud_network_instance.NET_INSTANCES_SWITCH_FIRST.name
  }

  interfaces {
    intfname    = zedcloud_application.UBUNTU_VM_DEF.manifest[0].interfaces[2].name
    intforder   = 3
    privateip   = false
    netinstname = zedcloud_network_instance.NET_INSTANCES_SWITCH_2ND.name
  }
}

output "EDGE_APP_INSTANCES" {
  description = "Print edge-app-instances which have been created for every edge-node which joined the project"
  sensitive   = true
  value = {
    id       = zedcloud_application_instance.APP_INSTANCES_VMS.id
    name     = zedcloud_application_instance.APP_INSTANCES_VMS.name
    password = random_password.vm_password.result
  }
}
