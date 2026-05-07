resource "zedcloud_brand" "SUPERMICRO" {
  name        = "SUPERMICRO_${var.config_suffix}"
  title       = "SUPERMICRO"
  origin_type = "ORIGIN_LOCAL"
}

resource "zedcloud_model" "SUPERMICRO_BLACK_TOWER" {
  name        = "SUPERMICRO_BLACK_TOWER_${var.config_suffix}"
  title       = "SUPERMICRO_BLACK_TOWER"
  origin_type = "ORIGIN_LOCAL"
  brand_id    = zedcloud_brand.SUPERMICRO.id
  attr = {
    "Cpus"    = "12"
    "memory"  = "65536M"
    "storage" = "500G"
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
      Ifname  = "eth0"
      PciLong = "0000:03:00.0"
    }
    phylabel     = "eth0"
    usage        = "ADAPTER_USAGE_APP_SHARED"
    usage_policy = {}
    ztype        = "IO_TYPE_ETH"
  }

  io_member_list {
    assigngrp    = "eth1"
    cbattr       = {}
    cost         = 0
    logicallabel = "eth1"
    phyaddrs = {
      Ifname  = "eth1"
      PciLong = "0000:03:00.1"
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
      Ifname  = "eth2"
      PciLong = "0000:06:00.0"
    }
    phylabel     = "eth2"
    usage        = "ADAPTER_USAGE_MANAGEMENT"
    usage_policy = {}
    ztype        = "IO_TYPE_ETH"
  }

  io_member_list {
    assigngrp    = "eth3"
    cbattr       = {}
    cost         = 0
    logicallabel = "eth3"
    phyaddrs = {
      Ifname  = "eth3"
      PciLong = "0000:06:00.1"
    }
    phylabel     = "eth3"
    usage        = "ADAPTER_USAGE_APP_SHARED"
    usage_policy = {}
    ztype        = "IO_TYPE_ETH_PF"
    vfs {
      count = 4
    }
  }
}
