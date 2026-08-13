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
  title       = "QEMU_VM with a single NIC, ${var.EDGE_NODE_CPUS} vCPUs and ${var.EDGE_NODE_MEM_GB}GB RAM"
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
}
