resource "zedcloud_brand" "qemu" {
  name        = "QEMU${local.us_name_suffix}"
  title       = "QEMU Brand"
  origin_type = "ORIGIN_LOCAL"
}

resource "zedcloud_model" "qemu_vm" {
  name        = "QEMU_VM${local.us_name_suffix}"
  title       = "QEMU VM Model with 4 ethernet interfaces"
  origin_type = "ORIGIN_LOCAL"
  brand_id    = zedcloud_brand.qemu.id

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

  io_member_list {
    assigngrp    = "eth1"
    cbattr       = {}
    cost         = 0
    logicallabel = "eth1"
    phyaddrs = {
      Ifname = "eth1"
    }
    phylabel     = "eth1"
    usage        = "ADAPTER_USAGE_APP_SHARED"
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
    usage        = "ADAPTER_USAGE_APP_SHARED"
    usage_policy = {}
    ztype        = "IO_TYPE_ETH"
  }

  io_member_list {
    assigngrp    = "eth3"
    cbattr       = {}
    cost         = 0
    logicallabel = "eth3"
    phyaddrs = {
      Ifname = "eth3"
    }
    phylabel     = "eth3"
    usage        = "ADAPTER_USAGE_APP_SHARED"
    usage_policy = {}
    ztype        = "IO_TYPE_ETH"
  }
}
