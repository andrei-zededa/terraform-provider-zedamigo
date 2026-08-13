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
  name  = "ENODE_TEST_${var.config_suffix}"
  title = "ENODE_TEST with many container app-instances"
  # Usually we would prefer to set a unique serial number like this and then
  # use it for the corresponding zedamigo_installed_edge_node and zedcloud_edgenode
  # resources as QEMU will set if through SMBIOS and then it will be detected by
  # EVE-OS as a "hardware serial number" (dmidecode system-serial-number). However
  # on macOS due to limitations of the Apple Virtualization Framework we cannot
  # set it, therefore we need to flip the logic. We let the EVE-OS install run
  # and generate a "soft serial" and use it here.
  # serialno       = "SN_TEST_${var.config_suffix}"
  serialno       = zedamigo_installed_edge_node.ENODE_TEST_INSTALL.soft_serial
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

  # A single management interface is enough here: all 27 edge-app-instances are
  # attached to one local (NAT-ed) network-instance which uses `uplink`, i.e.
  # this port, both for their outbound traffic and for the inbound port maps.
  interfaces {
    intfname   = "eth0"
    intf_usage = "ADAPTER_USAGE_MANAGEMENT"
    net_dhcp   = "NETWORK_DHCP_TYPE_CLIENT"
    cost       = 0
    netname    = zedcloud_network.edge_node_as_dhcp_client.name
    ztype      = "IO_TYPE_ETH"
    tags       = {}
  }

  tags = {}
}

#### This creates a QCOW2 disk image file which will be used for running the
#### QEMU VM with EVE-OS. It needs to be large enough for the EVE-OS partitions,
#### the container images of all the workloads and all of the volumes which the
#### 27 edge-app-instances ask for (43GB, see `workloads.tf`).
resource "zedamigo_disk_image" "empty_disk" {
  name    = "empty_disk_${var.config_suffix}"
  size_mb = var.EDGE_NODE_DISK_MB
}

#### This creates a custom EVE-OS installer ISO, it basically runs
#### `docker run ... lfedge/eve installer_iso`.
resource "zedamigo_eve_installer" "eve_os_installer" {
  name            = "EVE-OS_kvm_${lower(var.EDGE_NODE_ARCH)}"
  tag             = "16.0.1-lts-kvm-${lower(var.EDGE_NODE_ARCH)}"
  cluster         = var.ZEDEDA_CLOUD_URL
  authorized_keys = var.edge_node_ssh_pub_key
  grub_cfg        = <<-EOF
   set_getty
   # We need to set the console to the serial port. Originally we were using the
   # emulated ISA serial port in QEMU which is then available to the Linux guest
   # (EVE-OS) as ttyS0, however on macOS (with vfkit) only virtio-serial is available
   # which will be hvc0. QEMU is now also switched to virtio-serial.
   # set_global dom0_extra_args "$dom0_extra_args console=ttyS0 hv_console=ttyS0 dom0_console=ttyS0"
   set_global dom0_extra_args "$dom0_extra_args console=hvc0 hv_console=hvc0 dom0_console=hvc0"
   EOF
}

#### This will start a QEMU VM with the EVE-OS installer ISO previously
#### created and run the install process.
resource "zedamigo_installed_edge_node" "ENODE_TEST_INSTALL" {
  name = "ENODE_TEST_INSTALL_${var.config_suffix}"
  # See comment for zedcloud_edgenode.ENODE_TEST.serialno .
  # serial_no       = zedcloud_edgenode.ENODE_TEST.serialno
  serial_no       = "1234567890"
  installer_iso   = zedamigo_eve_installer.eve_os_installer.filename
  disk_image_base = zedamigo_disk_image.empty_disk.filename
}

#### This starts a QEMU VM with the disk onto which EVE-OS was installed basically
#### the zedamigo_installed_edge_node resource. The QEMU VM will be listening onto
#### a random port on `localhost` to allow for SSH access to EVE-OS. Find the port
#### with `tofu state show zedamigo_edge_node.ENODE_TEST_VM` and look at
#### `ssh_port` / `nic0_port_forwards`. Also `serial_console_log` is all the
#### output produced by the VM on it's serial console.
####
#### Note that this is a *big* VM: with the defaults of `vars.tf` it asks for
#### 80 vCPUs and 256GB of RAM, so the host has to be sized accordingly. See the
#### comment in `terraform.tf` about running this against a remote host.
resource "zedamigo_edge_node" "ENODE_TEST_VM" {
  name = "ENODE_TEST_VM_${var.config_suffix}"
  cpus = var.EDGE_NODE_CPUS
  mem  = "${var.EDGE_NODE_MEM_GB}G"
  # See comment for zedcloud_edgenode.ENODE_TEST.serialno .
  serial_no          = zedamigo_installed_edge_node.ENODE_TEST_INSTALL.serial_no
  serial_port_server = true
  disk_image_base    = zedamigo_installed_edge_node.ENODE_TEST_INSTALL.disk_image
  ovmf_vars_src      = zedamigo_installed_edge_node.ENODE_TEST_INSTALL.ovmf_vars
}
