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

resource "zedcloud_edgenode" "ENODE_TEST_AAAA" {
  name           = "ENODE_TEST_AAAA_${var.config_suffix}"
  title          = "ENODE_TEST AAAA"
  serialno       = "SN_TEST_AAAA_${var.config_suffix}"
  onboarding_key = var.onboarding_key
  model_id       = zedcloud_model.QEMU_ARM_VM.id
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

resource "zedcloud_edgenode" "ENODE_TEST_BBBB" {
  name           = "ENODE_TEST_BBBB_${var.config_suffix}"
  title          = "ENODE_TEST BBBB"
  serialno       = "SN_TEST_BBBB_${var.config_suffix}"
  onboarding_key = var.onboarding_key
  model_id       = zedcloud_model.QEMU_ARM_VM.id
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

#### This creates a custom EVE-OS installer ISO, it basically runs
#### `docker run ... lfedge/eve installer_iso`.
resource "zedamigo_eve_installer" "eve_os_installer_iso_1343" {
  name            = "EVE-OS_13.4.3-lts-kvm-arm64"
  tag             = "13.4.3-lts-kvm-arm64"
  cluster         = var.ZEDEDA_CLOUD_URL
  authorized_keys = var.edge_node_ssh_pub_key
  grub_cfg        = <<-EOF
   set_getty
   # This is actually better for the QEMU VM case.
   set_global dom0_extra_args "$dom0_extra_args console=ttyS0 hv_console=ttyS0 dom0_console=ttyS0"
   EOF
}

resource "zedamigo_eve_installer" "eve_os_installer_iso_1450" {
  name            = "EVE-OS_14.5.0-lts-kvm-arm64"
  tag             = "14.5.0-lts-kvm-arm64"
  cluster         = var.ZEDEDA_CLOUD_URL
  authorized_keys = var.edge_node_ssh_pub_key
  grub_cfg        = <<-EOF
   set_getty
   # This is actually better for the QEMU VM case.
   set_global dom0_extra_args "$dom0_extra_args console=ttyS0 hv_console=ttyS0 dom0_console=ttyS0"
   EOF
}

#### This will start a QEMU VM with the EVE-OS installer ISO previously
#### created and run the install process.
resource "zedamigo_installed_edge_node" "ENODE_TEST_INSTALL_AAAA" {
  name            = "ENODE_TEST_INSTALL_AAAA_${var.config_suffix}"
  serial_no       = zedcloud_edgenode.ENODE_TEST_AAAA.serialno
  installer_iso   = zedamigo_eve_installer.eve_os_installer_iso_1343.filename
  disk_image_base = zedamigo_disk_image.empty_disk_100G.filename
}

resource "zedamigo_installed_edge_node" "ENODE_TEST_INSTALL_BBBB" {
  name            = "ENODE_TEST_INSTALL_BBBB_${var.config_suffix}"
  serial_no       = zedcloud_edgenode.ENODE_TEST_BBBB.serialno
  installer_iso   = zedamigo_eve_installer.eve_os_installer_iso_1450.filename
  disk_image_base = zedamigo_disk_image.empty_disk_100G.filename
}

#### This starts a QEMU VM with the disk onto which EVE-OS was installed basically
#### the zedamigo_installed_edge_node resource. The QEMU VM will be listening onto
#### a random port on `localhost` to allow for SSH access to EVE-OS. Find the port
#### with:
#
#      ❯ tofu state show zedamigo_edge_node.ENODE_TEST_VM
#      # zedamigo_edge_node.ENODE_TEST_VM:
#      resource "zedamigo_edge_node" "ENODE_TEST_VM" {
#          cpus               = "4"
#          disk_image         = "/home/ev-zed1/.local/state/zedamigo/edge_nodes/f8086b9b-bfb5-4d11-8c70-77d4d0453e33/disk0.disk_img.qcow2"
#          disk_image_base    = "/home/ev-zed1/.local/state/zedamigo/installed_nodes/b99f1fae-3f51-4bda-933e-f9d29f01d857/disk0.disk_img.qcow2"
#          id                 = "f8086b9b-bfb5-4d11-8c70-77d4d0453e33"
#          mem                = "4G"
#          name               = "ENODE_TEST_VM_27791"
#          ovmf_vars          = "/home/ev-zed1/.local/state/zedamigo/edge_nodes/f8086b9b-bfb5-4d11-8c70-77d4d0453e33/UEFI_OVMF_VARS.bin"
#          ovmf_vars_src      = "/home/ev-zed1/.local/state/zedamigo/installed_nodes/b99f1fae-3f51-4bda-933e-f9d29f01d857/UEFI_OVMF_VARS.bin"
#          qmp_socket         = "unix:/home/ev-zed1/.local/state/zedamigo/edge_nodes/f8086b9b-bfb5-4d11-8c70-77d4d0453e33/qmp.socket,server,nowait"
#          serial_console_log = "/home/ev-zed1/.local/state/zedamigo/edge_nodes/f8086b9b-bfb5-4d11-8c70-77d4d0453e33/serial_console_run.log"
#          serial_no          = "SN_TEST_27791"
#          serial_port_server = true
#          serial_port_socket = "/home/ev-zed1/.local/state/zedamigo/edge_nodes/f8086b9b-bfb5-4d11-8c70-77d4d0453e33/serial_port.socket"
#          ssh_port           = 50277
#          vm_running         = true
#      }
#
#### `ssh_port` is the value. Also `serial_console_log` is all the output
#### produced by VM on it's serial console.
resource "zedamigo_edge_node" "ENODE_TEST_VM_AAAA" {
  name               = "ENODE_TEST_VM_AAAA_${var.config_suffix}"
  cpus               = "2"
  mem                = "2G"
  serial_no          = zedamigo_installed_edge_node.ENODE_TEST_INSTALL_AAAA.serial_no
  serial_port_server = true
  disk_image_base    = zedamigo_installed_edge_node.ENODE_TEST_INSTALL_AAAA.disk_image
  ovmf_vars_src      = zedamigo_installed_edge_node.ENODE_TEST_INSTALL_AAAA.ovmf_vars
}

resource "zedamigo_edge_node" "ENODE_TEST_VM_BBBB" {
  name               = "ENODE_TEST_VM_BBBB_${var.config_suffix}"
  cpus               = "2"
  mem                = "2G"
  serial_no          = zedamigo_installed_edge_node.ENODE_TEST_INSTALL_BBBB.serial_no
  serial_port_server = true
  disk_image_base    = zedamigo_installed_edge_node.ENODE_TEST_INSTALL_BBBB.disk_image
  ovmf_vars_src      = zedamigo_installed_edge_node.ENODE_TEST_INSTALL_BBBB.ovmf_vars
}
