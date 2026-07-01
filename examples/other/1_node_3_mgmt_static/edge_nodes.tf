variable "onboarding_key" {
  description = "Zedcloud onboarding key"
  type        = string
  default     = "5d0767ee-0547-4569-b530-387e526f8cb9"
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

  # eth0: the "default nic0" of the QEMU VM, used as the primary management
  # uplink. It is a DHCP client towards the QEMU embedded DHCP server + NAT and
  # is the interface that actually reaches the controller / the internet.
  interfaces {
    intfname   = "eth0"
    intf_usage = "ADAPTER_USAGE_MANAGEMENT"
    net_dhcp   = "NETWORK_DHCP_TYPE_CLIENT"
    cost       = 0
    netname    = zedcloud_network.edge_node_as_dhcp_client.name
    ztype      = "IO_TYPE_ETH"
    tags       = {}
  }

  # eth1: extra management NIC with a static IPv4 address. The subnet/gateway
  # come from the network object, the per-interface address from `ipaddr`. On
  # the host this NIC is connected to BRIDGE_1 (10.99.61.1/24).
  interfaces {
    intfname   = "eth1"
    intf_usage = "ADAPTER_USAGE_MANAGEMENT"
    net_dhcp   = "NETWORK_DHCP_TYPE_STATIC"
    ipaddr     = "10.99.61.10"
    cost       = 0
    netname    = zedcloud_network.mgmt_static_eth1.name
    ztype      = "IO_TYPE_ETH"
    tags       = {}
  }

  # eth2: extra management NIC with a static IPv6 address. Same as eth1 but with
  # an IPv6 address instead of IPv4: `net_dhcp` is still STATIC and the address
  # comes from `ipaddr`, only now it is an IPv6 address out of the network
  # object's ULA prefix. On the host this NIC is connected to BRIDGE_2
  # (fd00:99:62::1/64).
  interfaces {
    intfname   = "eth2"
    intf_usage = "ADAPTER_USAGE_MANAGEMENT"
    net_dhcp   = "NETWORK_DHCP_TYPE_STATIC"
    ipaddr     = "fd00:99:62::10"
    cost       = 0
    netname    = zedcloud_network.mgmt_static_eth2.name
    ztype      = "IO_TYPE_ETH"
    tags       = {}
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
#### networking (NAT + DHCP). The two extra NICs added below become eth1 and
#### eth2 and are connected to the host TAPs from host_networking.tf.
resource "zedamigo_edge_node" "ENODE_TEST_VM" {
  name               = "ENODE_TEST_VM_${var.config_suffix}"
  cpus               = 4
  mem                = "4G"
  serial_no          = zedamigo_installed_edge_node.ENODE_TEST_INSTALL.serial_no
  serial_port_server = true
  disk_image_base    = zedamigo_installed_edge_node.ENODE_TEST_INSTALL.disk_image
  ovmf_vars_src      = zedamigo_installed_edge_node.ENODE_TEST_INSTALL.ovmf_vars

  extra_qemu_args = [
    # eth1: static-IPv4 management NIC, backed by the host TAP on BRIDGE_1.
    "-nic", "tap,id=vmnet1,ifname=${zedamigo_tap.TAP_1.name},script=no,downscript=no,model=virtio",
    # eth2: static-IPv6 management NIC, backed by the host TAP on BRIDGE_2.
    "-nic", "tap,id=vmnet2,ifname=${zedamigo_tap.TAP_2.name},script=no,downscript=no,model=virtio",
  ]
}
