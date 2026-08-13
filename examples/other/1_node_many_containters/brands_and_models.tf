resource "zedcloud_brand" "QEMU" {
  name        = "QEMU_TEST_${var.config_suffix}"
  title       = "QEMU"
  origin_type = "ORIGIN_LOCAL"
}

# Unlike in most of the other examples the model attributes here are derived
# from the same variables which size the QEMU VM, so that what Zedcloud believes
# the hardware to be matches what EVE-OS actually finds. Zedcloud does not
# enforce anything based on these, but the edge-node resource usage views are a
# lot more useful when the model is not lying about the size of the node.
resource "zedcloud_model" "QEMU_VM" {
  name        = "QEMU_VM_TEST_${var.config_suffix}"
  title       = "QEMU_VM with ${length(local.EXTRA_MGMT_PORTS) + 1} NICs, ${var.EDGE_NODE_CPUS} vCPUs and ${var.EDGE_NODE_MEM_GB}GB RAM"
  origin_type = "ORIGIN_LOCAL"
  brand_id    = zedcloud_brand.QEMU.id
  attr = {
    "Cpus"    = tostring(var.EDGE_NODE_CPUS)
    "memory"  = "${var.EDGE_NODE_MEM_GB * 1024}M"
    "storage" = "${floor(var.EDGE_NODE_DISK_MB / 1000)}G"
  }
  product_status = "production"
  state          = "SYS_MODEL_STATE_ACTIVE"
  type           = upper(var.EDGE_NODE_ARCH)

  # eth0: the QEMU user-mode ("SLIRP") NIC, the only port of this edge-node
  # which actually reaches the controller, hence cost 0.
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

  # eth1..eth9: the nine extra management ports. Zedcloud only accepts an
  # `interfaces` entry on an edge-node for a port which its model declares, so
  # the model has to be kept in step with `local.EXTRA_MGMT_PORTS`
  # (host_networking.tf) — which is why both are generated from it.
  #
  # `for_each` over a map iterates in key order, so the members are emitted as
  # eth1, eth2, ... every time and the list does not churn between plans.
  dynamic "io_member_list" {
    for_each = local.EXTRA_MGMT_PORTS

    content {
      assigngrp    = io_member_list.key
      cbattr       = {}
      cost         = io_member_list.value.cost
      logicallabel = io_member_list.key
      phyaddrs = {
        Ifname = io_member_list.key
      }
      phylabel     = io_member_list.key
      usage        = "ADAPTER_USAGE_MANAGEMENT"
      usage_policy = {}
      ztype        = "IO_TYPE_ETH"
    }
  }
}
