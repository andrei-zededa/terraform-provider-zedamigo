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

  interfaces {
    intfname   = "eth1"
    intf_usage = "ADAPTER_USAGE_APP_SHARED"
    cost       = 0
    netname    = zedcloud_network.edge_node_as_dhcp_client.name
    # ztype      = "IO_TYPE_ETH"
    tags = {}
  }

  tags = {}
}

resource "zedamigo_eve_installer" "eve_os_installer_iso_1451" {
  name            = "EVE-OS_14.5.1-lts-kvm-amd64"
  tag             = "14.5.1-lts-kvm-amd64"
  cluster         = var.ZEDEDA_CLOUD_URL
  authorized_keys = var.edge_node_ssh_pub_key
  grub_cfg        = <<-EOF
   set_getty
   # This is actually better for the QEMU VM case.
   set_global dom0_extra_args "$dom0_extra_args console=ttyS0 hv_console=ttyS0 dom0_console=ttyS0"
   EOF
}

resource "zedamigo_disk_image" "empty_disk_100G" {
  name    = "empty_disk_100G"
  size_mb = 100000 # ~100GB
}

resource "zedamigo_installed_edge_node" "ENODE_TEST_INSTALL" {
  for_each = local.nodes

  name            = "ENODE_TEST_INSTALL_${each.value}_${var.config_suffix}"
  serial_no       = zedcloud_edgenode.ENODE_TEST[each.key].serialno
  installer_iso   = zedamigo_eve_installer.eve_os_installer_iso_1451.filename
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

  extra_qemu_args = [
    "-nic", "tap,id=vmnet1,ifname=${zedamigo_tap.TAP[each.key].name},script=no,downscript=no,model=e1000,mac=8c:84:74:11:01:01",
  ]

}
