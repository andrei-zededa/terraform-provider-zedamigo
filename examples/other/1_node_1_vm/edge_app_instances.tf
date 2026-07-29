resource "random_password" "vm_password" {
  length  = 10
  special = false
}

locals {
  nodes = {
    "ENODE_TEST_AAAA" = zedcloud_edgenode.ENODE_TEST_AAAA
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

  # Pin an explicit subnet/gateway instead of letting EVE-OS auto-assign one.
  # We need a stable, known gateway IP (the network-instance bridge IP) so that
  # the static route below can point at it (see the static_routes comment). The
  # bridge itself acts as the gateway, DNS server and DHCP server for the app.
  ip {
    subnet  = "10.166.0.0/16"
    gateway = "10.166.0.1"
    dns     = ["10.166.0.1"]
    dhcp_range {
      start = "10.166.0.128"
      end   = "10.166.255.254"
    }
  }

  # Advertise an explicit host route for the EVE-OS metadata server
  # (169.254.169.254) to the app through this local network-instance.
  #
  # The app is attached to several network-instances at once: this local NAT one
  # plus the switch network-instances on eth1/eth2. Each of them advertises its
  # own default route to the app, so the app ends up with multiple default
  # routes. Without a more specific route, the app has no way of knowing that
  # 169.254.169.254 must be reached specifically via this local network-instance:
  # it may pick one of the switch network-instances instead, where nothing
  # answers on 169.254.169.254 (EVE-OS only runs the metadata server on local
  # network-instances), and the metadata request therefore fails.
  #
  # This /32 route is more specific than any default route, so once it is
  # advertised the app always sends 169.254.169.254 traffic through this local
  # network-instance where the metadata server is reachable. We set `gateway` to
  # this network-instance's own gateway (bridge) IP: because the gateway is
  # inside the network-instance subnet, EVE-OS propagates the route verbatim to
  # the app via DHCP option 121 (`169.254.169.254/32 via 10.166.0.1`). This is
  # deterministic and does not depend on the uplink port having a resolvable
  # gateway, unlike the `output_port` form which is silently dropped when
  # EVE-OS cannot resolve a connected gateway for the output port.
  static_routes {
    prefix  = "169.254.169.254/32"
    gateway = "10.166.0.1"
  }

  tags = {
    ni_local_nat = "true"
  }
}

resource "zedcloud_network_instance" "NET_INSTANCES_SWITCH_ETH1" {
  for_each = local.nodes

  name      = "ni_switch_eth1_${each.value.name}_${var.config_suffix}"
  title     = "TF auto-created instance switch (port = eth1) for ${each.value.name}"
  kind      = "NETWORK_INSTANCE_KIND_SWITCH"
  type      = "NETWORK_INSTANCE_DHCP_TYPE_UNSPECIFIED"
  device_id = each.value.id

  port = "eth1"
}

resource "zedcloud_network_instance" "NET_INSTANCES_SWITCH_ETH2" {
  for_each = local.nodes

  name      = "ni_switch_eth2_${each.value.name}_${var.config_suffix}"
  title     = "TF auto-created instance switch (port = eth2) for ${each.value.name}"
  kind      = "NETWORK_INSTANCE_KIND_SWITCH"
  type      = "NETWORK_INSTANCE_DHCP_TYPE_UNSPECIFIED"
  device_id = each.value.id

  port = "eth2"
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

resource "zedcloud_volume_instance" "APP_PERSIST_STORAGE" {
  for_each = local.nodes

  name  = "app_persist_storage_on_${each.value.name}_${var.config_suffix}"
  title = "app_persist_storage_on_${each.value.name}_${var.config_suffix}"

  project_id = zedcloud_project.PROJECT.id
  device_id  = each.value.id

  type       = "VOLUME_INSTANCE_TYPE_BLOCKSTORAGE"
  accessmode = "VOLUME_INSTANCE_ACCESS_MODE_READWRITE"
  size_bytes = 21474836480 #### 20GB

  image       = ""
  multiattach = false
  cleartext   = true

  # Take the label directly from the edge-app definition.
  label = zedcloud_application.UBUNTU_VM_DEF.manifest[0].images[1].volumelabel
}

resource "zedcloud_application_instance" "APP_INSTANCES_VMS" {
  for_each = local.nodes

  # If we don't explictly set the dependency then the app-instances might be
  # created before the volume-instances exist on the node(s).
  depends_on = [zedcloud_volume_instance.APP_PERSIST_STORAGE]

  name      = "ubuntu_test_on_${each.value.name}"
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
    cpus = 6
    mode = zedcloud_application.UBUNTU_VM_DEF.manifest[0].vmmode
    vnc  = true
  }

  interfaces {
    intfname    = zedcloud_application.UBUNTU_VM_DEF.manifest[0].interfaces[0].name
    intforder   = 1
    privateip   = false
    netinstname = zedcloud_network_instance.NET_INSTANCES_APP_NAT[each.key].name
  }

  interfaces {
    intfname    = zedcloud_application.UBUNTU_VM_DEF.manifest[0].interfaces[1].name
    intforder   = 2
    privateip   = false
    netinstname = zedcloud_network_instance.NET_INSTANCES_SWITCH_ETH1[each.key].name
  }

  interfaces {
    intfname    = zedcloud_application.UBUNTU_VM_DEF.manifest[0].interfaces[2].name
    intforder   = 3
    privateip   = false
    netinstname = zedcloud_network_instance.NET_INSTANCES_SWITCH_ETH2[each.key].name
  }
}

output "EDGE_APP_INSTANCES" {
  description = "Print edge-app-instances which have been created for every edge-node which joined the project"
  sensitive   = true
  value = {
    for x in zedcloud_application_instance.APP_INSTANCES_VMS : x.name => {
      id       = x.id
      password = random_password.vm_password.result
    }
  }
}
