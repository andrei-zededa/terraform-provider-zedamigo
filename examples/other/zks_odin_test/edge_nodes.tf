variable "onboarding_key" {
  description = "Zedcloud onboarding key"
  type        = string
  default     = "5d0767ee-0547-4569-b530-387e526f8cb9"
}

resource "zedcloud_network" "edge_node_as_dhcp_client" {
  name  = "edge_node_as_dhcp_client_${var.config_suffix}"
  title = "edge_node_as_dhcp_client"
  kind  = "NETWORK_KIND_V4"

  project_id = zedcloud_project.PROJECT.id

  ip {
    dhcp = "NETWORK_DHCP_TYPE_CLIENT"
  }
  mtu = 1500
}

locals {
  nodes = {
    "ENODE_TEST_AAAA" = "AAAA"
    "ENODE_TEST_BBBB" = "BBBB"
    "ENODE_TEST_CCCC" = "CCCC"
  }
}

resource "zedcloud_edgenode" "ENODE_TEST" {
  for_each = local.nodes

  name           = "ENODE_TEST_${each.value}_${var.config_suffix}"
  title          = "ENODE_TEST ${each.value}"
  serialno       = "SN_TEST_${each.value}_${var.config_suffix}"
  onboarding_key = var.onboarding_key
  model_id       = zedcloud_model.QEMU_VM.id
  project_id     = zedcloud_project.PROJECT.id
  admin_state    = "ADMIN_STATE_ACTIVE"

  config_item {
    key          = "debug.enable.ssh"
    string_value = var.edge_node_ssh_pub_key
    # Need to set this otherwise we keep getting diff with the info in Zedcloud.
    uint64_value = "0"
  }

  interfaces {
    intfname   = "eth0"
    intf_usage = "ADAPTER_USAGE_MANAGEMENT"
    cost       = 0
    netname    = zedcloud_network.edge_node_as_dhcp_client.name
    # ztype      = "IO_TYPE_ETH"
    tags = {}
  }

  tags = {}
}

#### This creates a QCOW2 disk image file which will be used for running the
#### QEMU VM with EVE-OS.
resource "zedamigo_disk_image" "empty_disk_100G" {
  name    = "empty_disk_100G"
  size_mb = 100000 # ~100GB
}

resource "zedamigo_installed_edge_node" "ENODE_TEST_INSTALL" {
  for_each = local.nodes

  name            = "ENODE_TEST_INSTALL_${each.value}_${var.config_suffix}"
  serial_no       = zedcloud_edgenode.ENODE_TEST[each.key].serialno
  installer       = "${path.module}/zedcloud-odin-installer.raw"
  disk_image_base = zedamigo_disk_image.empty_disk_100G.filename
}

resource "zedamigo_edge_node" "ENODE_TEST_VM" {
  for_each = local.nodes

  name               = "ENODE_TEST_VM_${each.value}_${var.config_suffix}"
  cpus               = "4"
  mem                = "4G"
  serial_no          = zedamigo_installed_edge_node.ENODE_TEST_INSTALL[each.key].serial_no
  serial_port_server = true
  disk_image_base    = zedamigo_installed_edge_node.ENODE_TEST_INSTALL[each.key].disk_image
  ovmf_vars_src      = zedamigo_installed_edge_node.ENODE_TEST_INSTALL[each.key].ovmf_vars
}
