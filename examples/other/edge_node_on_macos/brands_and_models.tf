resource "zedcloud_brand" "QEMU_ARM" {
  name        = "QEMU_ARM_TEST_${var.config_suffix}"
  title       = "QEMU on ARM64"
  origin_type = "ORIGIN_LOCAL"
}

resource "zedcloud_model" "QEMU_ARM_VM" {
  name        = "QEMU_ARM_VM_TEST_${var.config_suffix}"
  title       = "QEMU ARM64 VM"
  origin_type = "ORIGIN_LOCAL"
  brand_id    = zedcloud_brand.QEMU_ARM.id
  attr = {
    "Cpus"    = "4"
    "memory"  = "4096M"
    "storage" = "100G"
  }
  product_status = "production"
  state          = "SYS_MODEL_STATE_ACTIVE"
  type           = "ARM64"

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
