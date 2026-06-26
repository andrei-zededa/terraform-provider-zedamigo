variable "onboarding_key" {
  description = "Zedcloud onboarding key"
  type        = string
  default     = "5d0767ee-0547-4569-b530-387e526f8cb9"
}

# Network object attached to the management NIC (eth0). The edge-node acts as a
# DHCP client on this port, getting its address from the QEMU embedded DHCP
# server (10.0.2.0/24) which also provides NAT to the outside.
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
    string_value = var.edge_node_ssh_pub_key
    # Need to set this otherwise we keep getting diff with the info in Zedcloud.
    uint64_value = "0"
  }

  # eth0: the "default nic0" of the QEMU VM, used as the management uplink. It
  # is a DHCP client towards the QEMU embedded DHCP server + NAT.
  interfaces {
    intfname   = "eth0"
    intf_usage = "ADAPTER_USAGE_MANAGEMENT"
    net_dhcp   = "NETWORK_DHCP_TYPE_CLIENT"
    cost       = 0
    netname    = zedcloud_network.edge_node_as_dhcp_client.name
    ztype      = "IO_TYPE_ETH"
    tags       = {}
  }

  # eth1: a VLANs-only trunk port. It carries no untagged network itself; VLAN
  # network-instances (e.g. tagged 506/507/508) are meant to be created on top
  # of it when deploying app-instances manually. On the host this NIC is the
  # TAP onto which VLANs 506/507/508 are created (see host_networking.tf).
  interfaces {
    intfname = "eth1"
    # Initially got started trying to use ADAPTER_USAGE_VLANS_ONLY but then
    # ran into an issue that the interface cannot be found while trying to
    # create a network-instance, and switched to ADAPTER_USAGE_APP_SHARED. But
    # that issue is more likely caused by a missing network object on the
    # interface, not by the "usage".
    intf_usage = "ADAPTER_USAGE_APP_SHARED"
    # With ADAPTER_USAGE_VLANS_ONLY and net. obj. config we get:
    #     Error: interface eth1 can't have netname/netid
    # net_dhcp   = "NETWORK_DHCP_TYPE_CLIENT"
    # netname    = zedcloud_network.edge_node_as_dhcp_client.name
    #
    # With ADAPTER_USAGE_VLANS_ONLY and without net. obj. config we get:
    #    Error: User defined port eth1 is assigned to VLANs only on device ....
    #
    # With ADAPTER_USAGE_APP_SHARED and without net. obj. config we get:
    #    network-instance in error state: no port is matching label 'eth1'
    #
    # Only with ADAPTER_USAGE_APP_SHARED and with net. obj. config it works.
    net_dhcp = "NETWORK_DHCP_TYPE_CLIENT"
    netname  = zedcloud_network.edge_node_as_dhcp_client.name
    cost     = 0
    ztype    = "IO_TYPE_ETH"
    tags     = {}
  }

  tags = {}
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

#### This creates a QCOW2 disk image file which will be used for running the
#### QEMU VM with EVE-OS.
resource "zedamigo_disk_image" "empty_disk" {
  name    = "empty_disk"
  size_mb = 100000 # ~100GB
}

#### This will start a QEMU VM with the EVE-OS installer ISO previously
#### created and run the install process.
resource "zedamigo_installed_edge_node" "ENODE_TEST_INSTALL" {
  name            = "ENODE_TEST_INSTALL_${var.config_suffix}"
  serial_no       = zedcloud_edgenode.ENODE_TEST.serialno
  installer_iso   = zedamigo_eve_installer.eve_os_installer.filename
  disk_image_base = zedamigo_disk_image.empty_disk.filename
}

#### This starts a QEMU VM with the disk onto which EVE-OS was installed. The
#### QEMU VM will be listening onto a random port on `localhost` to allow for
#### SSH access to EVE-OS. Find the port with:
####     tofu state show zedamigo_edge_node.ENODE_TEST_VM
#### and look at the `ssh_port` value.
####
#### eth0 (the "default nic0") is automatically backed by QEMU user-mode
#### networking (NAT + DHCP). The single extra NIC added below becomes eth1, the
#### VLANs-only trunk, and is connected to the host TAP from host_networking.tf.
resource "zedamigo_edge_node" "ENODE_TEST_VM" {
  name               = "ENODE_TEST_VM_${var.config_suffix}"
  cpus               = 4
  mem                = "4G"
  serial_no          = zedamigo_installed_edge_node.ENODE_TEST_INSTALL.serial_no
  serial_port_server = true
  disk_image_base    = zedamigo_installed_edge_node.ENODE_TEST_INSTALL.disk_image
  ovmf_vars_src      = zedamigo_installed_edge_node.ENODE_TEST_INSTALL.ovmf_vars

  extra_qemu_args = [
    # eth1: VLANs-only trunk, backed by the host TAP which carries VLANs
    # 506/507/508.
    "-nic", "tap,id=vmnet1,ifname=${zedamigo_tap.TAP_TRUNK.name},script=no,downscript=no,model=virtio",
  ]
}
