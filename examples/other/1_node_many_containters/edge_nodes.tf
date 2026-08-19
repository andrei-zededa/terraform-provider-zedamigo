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

  # eth0: the QEMU user-mode ("SLIRP") NIC. It is the only port of this
  # edge-node which actually reaches the controller and the internet, so it is
  # the cheapest one and it is what `uplink` — the port of the local (NAT-ed)
  # network-instance shared by all 27 edge-app-instances — resolves to for both
  # their outbound traffic and their inbound port maps.
  interfaces {
    intfname   = "eth0"
    intf_usage = "ADAPTER_USAGE_MANAGEMENT"
    net_dhcp   = "NETWORK_DHCP_TYPE_CLIENT"
    cost       = 0
    netname    = zedcloud_network.edge_node_as_dhcp_client.name
    ztype      = "IO_TYPE_ETH"
    tags       = {}
  }

  # eth1..eth9: nine more management ports, each one a DHCP client of the DHCP
  # server running in its own host network namespace (host_networking.tf). They
  # share the one `NETWORK_KIND_V4` / DHCP-client network object above — it
  # carries no addressing, so there is nothing per-port in it to differ.
  dynamic "interfaces" {
    for_each = local.EXTRA_MGMT_PORTS

    content {
      intfname   = interfaces.key
      intf_usage = "ADAPTER_USAGE_MANAGEMENT"
      net_dhcp   = "NETWORK_DHCP_TYPE_CLIENT"
      cost       = interfaces.value.cost
      netname    = zedcloud_network.edge_node_as_dhcp_client.name
      ztype      = "IO_TYPE_ETH"
      tags       = {}
    }
  }

  tags = {}
}

resource "zedamigo_host_reservation" "node_resources" {
  cpus = var.EDGE_NODE_CPUS
  mem  = var.EDGE_NODE_MEM_GB
  # devs = []
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
  tag             = "17.0.0-lts-kvm-${lower(var.EDGE_NODE_ARCH)}"
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
  name     = "ENODE_TEST_VM_${var.config_suffix}"
  cpus     = zedamigo_host_reservation.node_resources.cpus_reserved_count
  cpu_pins = zedamigo_host_reservation.node_resources.cpus_reserved
  mem      = "${zedamigo_host_reservation.node_resources.mem_reserved_total_gb}G"
  # See comment for zedcloud_edgenode.ENODE_TEST.serialno .
  serial_no          = zedamigo_installed_edge_node.ENODE_TEST_INSTALL.serial_no
  serial_port_server = true
  disk_image_base    = zedamigo_installed_edge_node.ENODE_TEST_INSTALL.disk_image
  ovmf_vars_src      = zedamigo_installed_edge_node.ENODE_TEST_INSTALL.ovmf_vars

  # eth1..eth9, each one backed by the TAP of its own host network namespace.
  #
  # The obvious spelling — one `-nic tap,...` per port — does not fit here.
  # `-nic` creates a *board* NIC, and a QEMU machine has only `MAX_NICS` == 8
  # of those slots; nic0 takes one, so the eighth TAP dies with:
  #
  #   -nic tap,id=vmnet8,...: no more on-board/default NIC slots available
  #
  # So the extra ports are `-netdev` + `-device` pairs instead, plugged into a
  # `pci-bridge` (32 slots) behind a `pcie-root-port` on q35's `pcie.0`. The
  # same topology as [`udp_traffic`](../udp_traffic/), which needs eight TAPs
  # of its own on top of nic0. In the guest:
  #
  #   -[0000:00]-+-00.0  82G33/G31/P35/P31 Express DRAM Controller
  #              +-02.0  Red Hat, Inc. Virtio network device          <- eth0
  #              +-10.0-[01-02]----00.0-[02]--+-01.0  Virtio network  <- eth1
  #              |                            +-02.0  Virtio network  <- eth2
  #              |                            \- ...                  <- ...
  #              \-1f.x  ICH9 LPC / SATA / SMBus
  #
  # Ordering still lands ethN on the port with index N, for two reasons: board
  # NICs are realized at machine init, i.e. before any `-device`, so nic0 stays
  # eth0 no matter where these appear on the command line; and the guest
  # enumerates the rest by PCI address, so pinning slot == port index (`addr`
  # 0x1..0x9) is what fixes the naming — not the argv order. `flatten` is
  # because each NIC is two argv entries.
  extra_qemu_args = concat(
    [
      "-device", "pcie-root-port,id=pcie1,bus=pcie.0,addr=0x10",
      "-device", "pci-bridge,id=pci1,bus=pcie1,chassis_nr=1",
    ],
    flatten([
      for name, port in local.EXTRA_MGMT_PORTS : [
        "-device",
        "virtio-net-pci,netdev=vmnet${port.index},mac=8c:84:74:10:99:${format("%02x", port.index)},bus=pci1,addr=${format("0x%x", port.index)}",
      ]
    ]),
    flatten([
      for name, port in local.EXTRA_MGMT_PORTS : [
        "-netdev",
        "tap,id=vmnet${port.index},ifname=${zedamigo_tap.MGMT[name].name},script=no,downscript=no",
      ]
    ]),
  )
}

locals {
  # `qmp_socket` is the value of the QEMU `-qmp` argument, e.g.
  # `unix:/var/lib/zedamigo/edge_nodes/<id>/qmp.socket,server,nowait`. Only the
  # socket path out of it is exported (`EDGE_NODE_QMP_SOCKET`), for poking the
  # VM by hand — e.g. a `set_link` to simulate a carrier loss on a NIC.
  #
  # An earlier version of this example had a `zedamigo_wait_until` barrier here
  # which took the link of eth0 — the only port with a path to the controller —
  # down as soon as the node reported in, to watch EVE-OS fail over across the
  # nine dead-end ports. That barrier is gone on purpose: this example now
  # reproduces the downloader crash (see the README), and for that the node has
  # to stay online and *download all 27 images*. Cutting the uplink before the
  # pulls start is exactly how the crash does NOT happen.
  QMP_SOCKET_PATH = trimsuffix(trimprefix(zedamigo_edge_node.ENODE_TEST_VM.qmp_socket, "unix:"), ",server,nowait")
}
