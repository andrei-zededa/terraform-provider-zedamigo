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
    assigngrp    = "group0"
    cbattr       = {}
    cost         = 0
    logicallabel = "port0"
    phyaddrs = {
      Ifname = "eth0"
    }
    phylabel     = "eth0"
    usage        = "ADAPTER_USAGE_MANAGEMENT"
    usage_policy = {}
    ztype        = "IO_TYPE_ETH"
  }

  io_member_list {
    assigngrp    = "group1"
    cbattr       = {}
    cost         = 0
    logicallabel = "port1"
    phyaddrs = {
      Ifname  = "eth1" # Matching just on PciLong doesn't work !
      PciLong = "0002:00.0"
    }
    phylabel     = "eth1"
    usage        = "ADAPTER_USAGE_APP_SHARED"
    usage_policy = {}
    ztype        = "IO_TYPE_ETH"
  }

  io_member_list {
    assigngrp    = "group2"
    cbattr       = {}
    cost         = 0
    logicallabel = "port2"
    phyaddrs = {
      Ifname  = "eth2" # Matching just on PciLong doesn't work !
      PciLong = "0002:01.0"
    }
    phylabel     = "eth2"
    usage        = "ADAPTER_USAGE_APP_SHARED"
    usage_policy = {}
    ztype        = "IO_TYPE_ETH"
  }

  io_member_list {
    assigngrp    = "group3"
    cbattr       = {}
    cost         = 0
    logicallabel = "port3"
    phyaddrs = {
      Ifname  = "eth3" # Matching just on PciLong doesn't work !
      PciLong = "0002:02.0"
    }
    phylabel     = "eth3"
    usage        = "ADAPTER_USAGE_APP_SHARED"
    usage_policy = {}
    ztype        = "IO_TYPE_ETH"
  }

  io_member_list {
    assigngrp    = "group4"
    cbattr       = {}
    cost         = 0
    logicallabel = "port4"
    phyaddrs = {
      Ifname  = "eth4" # Matching just on PciLong doesn't work !
      PciLong = "0002:02.0"
    }
    phylabel     = "eth4"
    usage        = "ADAPTER_USAGE_APP_SHARED"
    usage_policy = {}
    ztype        = "IO_TYPE_ETH"
  }
}
