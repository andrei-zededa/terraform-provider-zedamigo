resource "zedcloud_brand" "QEMU" {
  name        = "QEMU_TEST_${var.config_suffix}"
  title       = "QEMU"
  origin_type = "ORIGIN_LOCAL"
}

resource "zedcloud_model" "QEMU_VM" {
  name        = "QEMU_VM_TEST_${var.config_suffix}"
  title       = "QEMU_VM_WITH_MANY_PORTS"
  origin_type = "ORIGIN_LOCAL"
  brand_id    = zedcloud_brand.QEMU.id
  attr = {
    "Cpus"    = "4"
    "memory"  = "4096M"
    "storage" = "100G"
  }
  product_status = "production"
  state          = "SYS_MODEL_STATE_ACTIVE"
  type           = "AMD64"

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
