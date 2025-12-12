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
  name           = "ENODE_TEST_${var.config_suffix}"
  title          = "ENODE_TEST"
  serialno       = "SN_TEST_${var.config_suffix}"
  onboarding_key = var.onboarding_key
  model_id       = zedcloud_model.QEMU_VM.id
  project_id     = zedcloud_project.PROJECT.id
  admin_state    = "ADMIN_STATE_ACTIVE"

  config_item {
    key          = "debug.enable.ssh"
    string_value = var.ssh_pub_key
    # Need to set this otherwise we keep getting diff with the info in Zedcloud.
    uint64_value = "0"
  }

  interfaces {
    intfname   = "eth0"
    intf_usage = "ADAPTER_USAGE_MANAGEMENT"
    cost       = 0
    netname    = zedcloud_network.edge_node_as_dhcp_client.name
    net_dhcp   = "NETWORK_DHCP_TYPE_CLIENT"
    ztype      = "IO_TYPE_ETH"

    tags = {}
  }
  tags = {}
}

resource "zedamigo_eve_installer" "eve_os_installer_iso" {
  name            = "EVE-OS"
  tag             = "14.5.2-lts-kvm-amd64"
  cluster         = var.ZEDEDA_CLOUD_URL
  authorized_keys = var.ssh_pub_key
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
  name            = "ENODE_TEST_INSTALL_${var.config_suffix}"
  serial_no       = zedcloud_edgenode.ENODE_TEST.serialno
  installer_iso   = zedamigo_eve_installer.eve_os_installer_iso.filename
  disk_image_base = zedamigo_disk_image.empty_disk_100G.filename
}

resource "zedamigo_edge_node" "ENODE_TEST_VM" {
  name               = "ENODE_TEST_VM_${var.config_suffix}"
  cpus               = 2
  mem                = "2G"
  serial_no          = zedamigo_installed_edge_node.ENODE_TEST_INSTALL.serial_no
  serial_port_server = true
  disk_image_base    = zedamigo_installed_edge_node.ENODE_TEST_INSTALL.disk_image
  ovmf_vars_src      = zedamigo_installed_edge_node.ENODE_TEST_INSTALL.ovmf_vars
}
