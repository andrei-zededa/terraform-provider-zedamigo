variable "onboarding_key" {
  description = "Zedcloud onboarding key"
  type        = string
  default     = "5d0767ee-0547-4569-b530-387e526f8cb9"
}

resource "zedcloud_network" "netobj_dual_ipv4_pref" {
  name  = "netobj_dual_ipv4_pref_${var.config_suffix}"
  title = "netobj_dual_ipv4_pref"
  kind  = "NETWORK_KIND_V4"

  project_id = zedcloud_project.PROJECT.id

  ip {
    dhcp = "NETWORK_DHCP_TYPE_CLIENT"
  }
  mtu = 1500
}

resource "zedcloud_network" "netobj_dual_ipv6_pref" {
  name  = "netobj_dual_ipv6_pref_${var.config_suffix}"
  title = "netobj_dual_ipv6_pref"
  kind  = "NETWORK_KIND_V6"

  project_id = zedcloud_project.PROJECT.id

  ip {
    dhcp = "NETWORK_DHCP_TYPE_CLIENT"
  }
  mtu = 1500
}

resource "zedcloud_network" "netobj_ipv4_only" {
  name  = "netobj_ipv4_only_${var.config_suffix}"
  title = "netobj_ipv4_only"
  kind  = "NETWORK_KIND_V4_ONLY"

  project_id = zedcloud_project.PROJECT.id

  ip {
    dhcp = "NETWORK_DHCP_TYPE_CLIENT"
  }
  mtu = 1500
}

resource "zedcloud_network" "netobj_ipv6_only" {
  name  = "netobj_ipv6_only_${var.config_suffix}"
  title = "netobj_ipv6_only"
  kind  = "NETWORK_KIND_V6_ONLY"

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
    intfname   = "port0"
    intf_usage = "ADAPTER_USAGE_MANAGEMENT"
    cost       = 0
    netname    = zedcloud_network.netobj_dual_ipv4_pref.name
    # ztype      = "IO_TYPE_ETH" # Not supported in 2.5.0, we need the next version.
    tags = {}
  }

  interfaces {
    intfname   = "port1"
    intf_usage = "ADAPTER_USAGE_APP_SHARED"
    cost       = 0
    netname    = zedcloud_network.netobj_dual_ipv4_pref.name
    # ztype      = "IO_TYPE_ETH" # Not supported in 2.5.0, we need the next version.
    tags = {}
  }

  interfaces {
    intfname   = "port2"
    intf_usage = "ADAPTER_USAGE_APP_SHARED"
    cost       = 0
    netname    = zedcloud_network.netobj_ipv4_only.name
    # ztype      = "IO_TYPE_ETH" # Not supported in 2.5.0, we need the next version.
    tags = {}
  }

  interfaces {
    intfname   = "port3"
    intf_usage = "ADAPTER_USAGE_APP_SHARED"
    cost       = 0
    netname    = zedcloud_network.netobj_dual_ipv6_pref.name
    # ztype      = "IO_TYPE_ETH" # Not supported in 2.5.0, we need the next version.
    tags = {}
  }

  interfaces {
    intfname   = "port4"
    intf_usage = "ADAPTER_USAGE_APP_SHARED"
    cost       = 0
    netname    = zedcloud_network.netobj_ipv6_only.name
    # ztype      = "IO_TYPE_ETH" # Not supported in 2.5.0, we need the next version.
    tags = {}
  }

  interfaces {
    intfname   = "port5"
    intf_usage = "ADAPTER_USAGE_APP_SHARED"
    cost       = 0
    netname    = zedcloud_network.netobj_dual_ipv6_pref.name
    # ztype      = "IO_TYPE_ETH" # Not supported in 2.5.0, we need the next version.
    tags = {}
  }

  interfaces {
    intfname   = "port6"
    intf_usage = "ADAPTER_USAGE_APP_SHARED"
    cost       = 0
    netname    = zedcloud_network.netobj_ipv6_only.name
    # ztype      = "IO_TYPE_ETH" # Not supported in 2.5.0, we need the next version.
    tags = {}
  }

  interfaces {
    intfname   = "port7"
    intf_usage = "ADAPTER_USAGE_APP_SHARED"
    cost       = 0
    # ztype      = "IO_TYPE_ETH" # Not supported in 2.5.0, we need the next version.
    tags = {}
  }

  interfaces {
    intfname   = "port8"
    intf_usage = "ADAPTER_USAGE_APP_SHARED"
    #### intf_usage = "ADAPTER_USAGE_VLANS_ONLY"
    cost = 0
    # ztype      = "IO_TYPE_ETH" # Not supported in 2.5.0, we need the next version.
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
resource "zedamigo_eve_installer" "eve_os_installer_iso" {
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

#### This will start a QEMU VM with the EVE-OS installer ISO previously
#### created and run the install process.
resource "zedamigo_installed_edge_node" "ENODE_TEST_INSTALL_AAAA" {
  name            = "ENODE_TEST_INSTALL_AAAA_${var.config_suffix}"
  serial_no       = zedcloud_edgenode.ENODE_TEST_AAAA.serialno
  installer_iso   = zedamigo_eve_installer.eve_os_installer_iso.filename
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

  extra_qemu_args = [
    # ''Simple'' way of adding more NICs:
    #
    #    "-nic", "tap,id=vmnet1,ifname=${zedamigo_tap.TAP_101.name},script=no,downscript=no,model=e1000,mac=8c:84:74:11:01:01",
    #    "-nic", "tap,id=vmnet2,ifname=${zedamigo_tap.TAP_102.name},script=no,downscript=no,model=e1000,mac=8c:84:74:11:01:02",
    #    "-nic", "tap,id=vmnet3,ifname=${zedamigo_tap.TAP_103.name},script=no,downscript=no,model=e1000,mac=8c:84:74:11:01:03",
    #
    # More ''complicated'' way which results in the following PCI topology:
    #
    #   49d1519f-8083-4016-b3c4-f6b0edfed7fe:~# lspci -tv
    #   -[0000:00]-+-00.0  Intel Corporation 82G33/G31/P35/P31 Express DRAM Controller
    #              +-01.0  Device 1234:1111
    #              +-02.0  Red Hat, Inc. Virtio network device
    #              +-10.0-[01-02]----00.0-[02]--+-00.0  Intel Corporation 82540EM Gigabit Ethernet Controller
    #              |                            +-01.0  Intel Corporation 82540EM Gigabit Ethernet Controller
    #              |                            +-02.0  Intel Corporation 82540EM Gigabit Ethernet Controller
    #              |                            \-03.0  Intel Corporation 82540EM Gigabit Ethernet Controller
    #              +-1f.0  Intel Corporation 82801IB (ICH9) LPC Interface Controller
    #              +-1f.2  Intel Corporation 82801IR/IO/IH (ICH9R/DO/DH) 6 port SATA Controller [AHCI mode]
    #              \-1f.3  Intel Corporation 82801I (ICH9 Family) SMBus Controller
    #   49d1519f-8083-4016-b3c4-f6b0edfed7fe:~# lspci -v | grep Ethernet
    #   00:02.0 Ethernet controller: Red Hat, Inc. Virtio network device
    #   02:00.0 Ethernet controller: Intel Corporation 82540EM Gigabit Ethernet Controller (rev 03)
    #   02:01.0 Ethernet controller: Intel Corporation 82540EM Gigabit Ethernet Controller (rev 03)
    #   02:02.0 Ethernet controller: Intel Corporation 82540EM Gigabit Ethernet Controller (rev 03)
    #   02:03.0 Ethernet controller: Intel Corporation 82540EM Gigabit Ethernet Controller (rev 03)
    #
    "-device", "pcie-root-port,id=pcie1,bus=pcie.0,addr=0x10",
    "-device", "pci-bridge,id=pci1,bus=pcie1,chassis_nr=1",
    "-device", "e1000,netdev=vmnet1,mac=8c:84:74:10:00:01,bus=pci1,addr=0x0",
    "-device", "e1000,netdev=vmnet2,mac=8c:84:74:10:00:02,bus=pci1,addr=0x1",
    "-device", "e1000,netdev=vmnet3,mac=8c:84:74:10:00:03,bus=pci1,addr=0x2",
    "-device", "e1000,netdev=vmnet4,mac=8c:84:74:10:00:04,bus=pci1,addr=0x3",
    "-device", "e1000,netdev=vmnet5,mac=8c:84:74:10:00:05,bus=pci1,addr=0x4",
    "-device", "e1000,netdev=vmnet6,mac=8c:84:74:10:00:06,bus=pci1,addr=0x5",
    "-device", "e1000,netdev=vmnet7,mac=8c:84:74:10:00:07,bus=pci1,addr=0x6",
    "-device", "e1000,netdev=vmnet8,mac=8c:84:74:10:00:08,bus=pci1,addr=0x7",
    "-netdev", "tap,id=vmnet1,ifname=${zedamigo_tap.TAP_101.name},script=no,downscript=no",
    "-netdev", "tap,id=vmnet2,ifname=${zedamigo_tap.TAP_102.name},script=no,downscript=no",
    "-netdev", "tap,id=vmnet3,ifname=${zedamigo_tap.TAP_103.name},script=no,downscript=no",
    "-netdev", "tap,id=vmnet4,ifname=${zedamigo_tap.TAP_104.name},script=no,downscript=no",
    "-netdev", "tap,id=vmnet5,ifname=${zedamigo_tap.TAP_105.name},script=no,downscript=no",
    "-netdev", "tap,id=vmnet6,ifname=${zedamigo_tap.TAP_106.name},script=no,downscript=no",
    "-netdev", "tap,id=vmnet7,ifname=${zedamigo_tap.TAP_107.name},script=no,downscript=no",
    "-netdev", "tap,id=vmnet8,ifname=${zedamigo_tap.TAP_108.name},script=no,downscript=no",
  ]
}
