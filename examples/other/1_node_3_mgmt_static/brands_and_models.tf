resource "zedcloud_brand" "QEMU" {
  name        = "QEMU_TEST_${var.config_suffix}"
  title       = "QEMU"
  origin_type = "ORIGIN_LOCAL"
}

# A model with 3 management NICs:
#   - eth0: the "default nic0" of the QEMU VM, backed by QEMU user-mode
#     networking (NAT + embedded DHCP server on 10.0.2.0/24). This is the
#     primary management/uplink port and the one that actually reaches the
#     controller / the internet.
#   - eth1, eth2: extra management NICs. On the host each is backed by a TAP
#     enslaved to its own bridge (see host_networking.tf). eth1 is configured
#     with a static IPv4 address and eth2 with a static IPv6 address (see
#     networks.tf / edge_nodes.tf).
#
# All three NICs use ADAPTER_USAGE_MANAGEMENT.
resource "zedcloud_model" "QEMU_VM" {
  name        = "QEMU_VM_TEST_${var.config_suffix}"
  title       = "QEMU_VM with 3 management NICs"
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
    phylabel     = "eth1"
    usage        = "ADAPTER_USAGE_MANAGEMENT"
    usage_policy = {}
    ztype        = "IO_TYPE_ETH"
  }

  io_member_list {
    assigngrp    = "eth2"
    cbattr       = {}
    cost         = 0
    logicallabel = "eth2"
    phyaddrs = {
      Ifname = "eth2"
    }
    phylabel     = "eth2"
    usage        = "ADAPTER_USAGE_MANAGEMENT"
    usage_policy = {}
    ztype        = "IO_TYPE_ETH"
  }
}
