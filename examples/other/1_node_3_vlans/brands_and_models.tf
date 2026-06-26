resource "zedcloud_brand" "QEMU" {
  name        = "QEMU_TEST_${var.config_suffix}"
  title       = "QEMU"
  origin_type = "ORIGIN_LOCAL"
}

# A model with 2 NICs:
#   - eth0: ADAPTER_USAGE_MANAGEMENT, the "default nic0" of the QEMU VM which is
#     backed by QEMU user-mode networking (NAT + embedded DHCP server). This is
#     the management/uplink port.
#   - eth1: ADAPTER_USAGE_VLANS_ONLY, a trunk port that only carries VLAN
#     sub-interfaces. On the host this NIC is backed by a TAP onto which VLANs
#     506, 507 & 508 are created (see host_networking.tf).
resource "zedcloud_model" "QEMU_VM" {
  name        = "QEMU_VM_TEST_${var.config_suffix}"
  title       = "QEMU_VM with a management NIC and a VLANs-only trunk NIC"
  origin_type = "ORIGIN_LOCAL"
  brand_id    = zedcloud_brand.QEMU.id
  attr = {
    "Cpus"    = "4"
    "memory"  = "4096M"
    "storage" = "100G"
  }
  product_status = "production"
  state          = "SYS_MODEL_STATE_ACTIVE"
  type           = upper(var.EDGE_NODE_ARCH)

  io_member_list {
    assigngrp    = "eth0"
    cbattr       = {}
    cost         = 0
    logicallabel = "eth0"
    phyaddrs = {
      Ifname = "eth0"
    }
    phylabel     = "eth0"
    usage        = "ADAPTER_USAGE_MANAGEMENT"
    usage_policy = {}
    ztype        = "IO_TYPE_ETH"
  }

  io_member_list {
    assigngrp    = "eth1"
    cbattr       = {}
    cost         = 0
    logicallabel = "eth1"
    phyaddrs = {
      Ifname = "eth1"
    }
    phylabel = "eth1"
    # Initially got started trying to use ADAPTER_USAGE_VLANS_ONLY but then
    # ran into an issue that the interface cannot be found while trying to
    # create a network-instance, and switched to ADAPTER_USAGE_APP_SHARED. But
    # that issue is more likely caused by a missing network object on the
    # interface, not by the "usage". See more details in edge_node.tf .
    usage        = "ADAPTER_USAGE_APP_SHARED"
    usage_policy = {}
    ztype        = "IO_TYPE_ETH"
  }
}
