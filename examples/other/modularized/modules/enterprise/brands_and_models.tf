resource "zedcloud_brand" "operational-services" {
  name        = "operational-services"
  origin_type = "ORIGIN_LOCAL"
  title       = "operational-services"
  logo = {
    url = "https://www.operational-services.de/typo3conf/ext/bb_themepackage_os/Resources/Public/Images/logo_blue.svg"
  }
}

resource "zedcloud_brand" "Axiomtek" {
  name        = "Axiomtek"
  origin_type = "ORIGIN_LOCAL"
  title       = "Axiomtek"
  logo = {
    url = "https://www.axiomtek.de/content/themes/axiomtek/build/assets/favicons/favicon.ico"
  }
}

resource "zedcloud_model" "os_EM321-102_TYPE-M-8TB" {
  name        = "os_EM321-102_TYPE-M-8TB"
  brand_id    = zedcloud_brand.operational-services.id
  description = "os_em321-102_TYPE-M-8TB-NVMe-WLAN-WWAN-SFPNIC"
  title       = "os_EM321-102_TYPE-M-8TB"

  attr = {
    Cpus     = "16"
    hsm      = "1"
    leds     = "0"
    memory   = "64G"
    storage  = "7168G"
    watchdog = "true"
  }

  origin_type    = "ORIGIN_LOCAL"
  product_status = "production"
  product_url    = ""
  state          = "SYS_MODEL_STATE_ACTIVE"
  type           = "AMD64"

  io_member_list {
    assigngrp       = "COM2"
    cbattr          = {}
    cost            = 0
    logicallabel    = "COM2"
    parentassigngrp = ""
    phyaddrs = {
      Ioports = "2f8-2ff"
      Irq     = "3"
      Serial  = "/dev/ttyS1"
    }
    phylabel     = "COM2"
    usage        = "ADAPTER_USAGE_UNSPECIFIED"
    usage_policy = {}
    ztype        = "IO_TYPE_COM"
  }

  io_member_list {
    assigngrp       = "group29"
    cbattr          = {}
    cost            = 0
    logicallabel    = "eth0"
    parentassigngrp = ""
    phyaddrs = {
      Ifname  = "eth0"
      PciLong = "0000:01:00.0"
    }
    phylabel     = "eth0"
    usage        = "ADAPTER_USAGE_MANAGEMENT"
    usage_policy = {}
    ztype        = "IO_TYPE_ETH"
  }

  io_member_list {
    assigngrp       = "group30"
    cbattr          = {}
    cost            = 0
    logicallabel    = "eth1"
    parentassigngrp = ""
    phyaddrs = {
      Ifname  = "eth1"
      PciLong = "0000:01:00.1"
    }
    phylabel     = "eth1"
    usage        = "ADAPTER_USAGE_MANAGEMENT"
    usage_policy = {}
    ztype        = "IO_TYPE_ETH"
  }

  io_member_list {
    assigngrp       = "group31"
    cbattr          = {}
    cost            = 0
    logicallabel    = "eth2"
    parentassigngrp = ""
    phyaddrs = {
      Ifname  = "eth2"
      PciLong = "0000:01:00.2"
    }
    phylabel     = "eth2"
    usage        = "ADAPTER_USAGE_MANAGEMENT"
    usage_policy = {}
    ztype        = "IO_TYPE_ETH"
  }

  io_member_list {
    assigngrp       = "group32"
    cbattr          = {}
    cost            = 0
    logicallabel    = "eth3"
    parentassigngrp = ""
    phyaddrs = {
      Ifname  = "eth3"
      PciLong = "0000:01:00.3"
    }
    phylabel     = "eth3"
    usage        = "ADAPTER_USAGE_MANAGEMENT"
    usage_policy = {}
    ztype        = "IO_TYPE_ETH"
  }

  io_member_list {
    assigngrp       = "group33"
    cbattr          = {}
    cost            = 0
    logicallabel    = "eth4"
    parentassigngrp = ""
    phyaddrs = {
      Ifname  = "eth4"
      PciLong = "0000:02:00.0"
    }
    phylabel     = "eth4"
    usage        = "ADAPTER_USAGE_MANAGEMENT"
    usage_policy = {}
    ztype        = "IO_TYPE_ETH"
  }

  io_member_list {
    assigngrp       = "group2"
    cbattr          = {}
    cost            = 0
    logicallabel    = "eth5"
    parentassigngrp = ""
    phyaddrs = {
      Ifname  = "eth5"
      PciLong = "0000:65:00.0"
    }
    phylabel     = "eth5"
    usage        = "ADAPTER_USAGE_MANAGEMENT"
    usage_policy = {}
    ztype        = "IO_TYPE_ETH"
  }

  io_member_list {
    assigngrp       = "group3"
    cbattr          = {}
    cost            = 0
    logicallabel    = "eth6"
    parentassigngrp = ""
    phyaddrs = {
      Ifname  = "eth6"
      PciLong = "0000:65:00.1"
    }
    phylabel     = "eth6"
    usage        = "ADAPTER_USAGE_MANAGEMENT"
    usage_policy = {}
    ztype        = "IO_TYPE_ETH"
  }

  io_member_list {
    assigngrp       = "group21"
    cbattr          = {}
    cost            = 0
    logicallabel    = "USB"
    parentassigngrp = ""
    phyaddrs = {
      PciLong = "0000:00:14.0"
    }
    phylabel     = "USB"
    usage        = "ADAPTER_USAGE_UNSPECIFIED"
    usage_policy = {}
    ztype        = "IO_TYPE_USB_CONTROLLER"
  }

  io_member_list {
    assigngrp       = "COM1"
    cbattr          = {}
    cost            = 0
    logicallabel    = "COM1"
    parentassigngrp = ""
    phyaddrs = {
      Ioports = "3f8-3ff"
      Irq     = "4"
      Serial  = "/dev/ttyS0"
    }
    phylabel     = "COM1"
    usage        = "ADAPTER_USAGE_UNSPECIFIED"
    usage_policy = {}
    ztype        = "IO_TYPE_COM"
  }

  io_member_list {
    assigngrp       = "group34"
    cbattr          = {}
    cost            = 0
    logicallabel    = "wlan0"
    parentassigngrp = ""
    phyaddrs = {
      Ifname  = "wlan0"
      PciLong = "0000:03:00.0"
    }
    phylabel     = "wlan0"
    usage        = "ADAPTER_USAGE_MANAGEMENT"
    usage_policy = {}
    ztype        = "IO_TYPE_WLAN"
  }

  io_member_list {
    assigngrp       = "group21"
    cbattr          = {}
    cost            = 10
    logicallabel    = "WWAN-2c7c:0512-2:6"
    parentassigngrp = ""
    phyaddrs = {
      usbaddr    = "2:6"
      usbproduct = "2c7c:0512"
    }
    phylabel     = "WWAN-2c7c:0512-2:6"
    usage        = "ADAPTER_USAGE_UNSPECIFIED"
    usage_policy = {}
    ztype        = "IO_TYPE_WWAN"
  }

  io_member_list {
    assigngrp       = "group35"
    cbattr          = {}
    cost            = 0
    logicallabel    = "VGA"
    parentassigngrp = ""
    phyaddrs = {
      PciLong = "0000:05:00.0"
    }
    phylabel     = "VGA"
    usage        = "ADAPTER_USAGE_UNSPECIFIED"
    usage_policy = {}
    ztype        = "IO_TYPE_HDMI"
  }
}

resource "zedcloud_model" "DSP-521" {
  name     = "DSP-521"
  title    = "DSP-521"
  brand_id = zedcloud_brand.Axiomtek.id

  attr = {
    Cpus     = "12"
    hsm      = "1"
    leds     = "0"
    memory   = "7431M"
    storage  = "119G"
    watchdog = "true"
  }

  origin_type    = "ORIGIN_LOCAL"
  product_status = "production"
  product_url    = ""
  state          = "SYS_MODEL_STATE_ACTIVE"
  type           = "AMD64"

  io_member_list {
    assigngrp       = "group0"
    cbattr          = {}
    cost            = 0
    logicallabel    = "VGA"
    parentassigngrp = ""
    phyaddrs = {
      PciLong = "0000:00:02.0"
    }
    phylabel     = "VGA"
    usage        = "ADAPTER_USAGE_UNSPECIFIED"
    usage_policy = {}
    ztype        = "IO_TYPE_HDMI"
  }

  io_member_list {
    assigngrp       = "COM1"
    cbattr          = {}
    cost            = 0
    logicallabel    = "COM1"
    parentassigngrp = ""
    phyaddrs = {
      Ioports = "3f8-3ff"
      Irq     = "4"
      Serial  = "/dev/ttyS0"
    }
    phylabel     = "COM1"
    usage        = "ADAPTER_USAGE_UNSPECIFIED"
    usage_policy = {}
    ztype        = "IO_TYPE_COM"
  }

  io_member_list {
    assigngrp       = "COM2"
    cbattr          = {}
    cost            = 0
    logicallabel    = "COM2"
    parentassigngrp = ""
    phyaddrs = {
      Ioports = "2f8-2ff"
      Irq     = "3"
      Serial  = "/dev/ttyS1"
    }
    phylabel     = "COM2"
    usage        = "ADAPTER_USAGE_UNSPECIFIED"
    usage_policy = {}
    ztype        = "IO_TYPE_COM"
  }

  io_member_list {
    assigngrp       = "group11"
    cbattr          = {}
    cost            = 0
    logicallabel    = "eth0"
    parentassigngrp = ""
    phyaddrs = {
      Ifname  = "eth0"
      PciLong = "0000:00:1f.6"
    }
    phylabel     = "eth0"
    usage        = "ADAPTER_USAGE_MANAGEMENT"
    usage_policy = {}
    ztype        = "IO_TYPE_ETH"
  }

  io_member_list {
    assigngrp       = "group12"
    cbattr          = {}
    cost            = 0
    logicallabel    = "eth1"
    parentassigngrp = ""
    phyaddrs = {
      Ifname  = "eth1"
      PciLong = "0000:01:00.0"
    }
    phylabel     = "eth1"
    usage        = "ADAPTER_USAGE_MANAGEMENT"
    usage_policy = {}
    ztype        = "IO_TYPE_ETH"
  }

  io_member_list {
    assigngrp       = "group11"
    cbattr          = {}
    cost            = 0
    logicallabel    = "Audio"
    parentassigngrp = ""
    phyaddrs = {
      PciLong = "0000:00:1f.3"
    }
    phylabel     = "Audio"
    usage        = "ADAPTER_USAGE_UNSPECIFIED"
    usage_policy = {}
    ztype        = "IO_TYPE_AUDIO"
  }

  io_member_list {
    assigngrp       = "group4"
    cbattr          = {}
    cost            = 0
    logicallabel    = "USB"
    parentassigngrp = ""
    phyaddrs = {
      PciLong = "0000:00:14.0"
    }
    phylabel     = "USB"
    usage        = "ADAPTER_USAGE_UNSPECIFIED"
    usage_policy = {}
    ztype        = "IO_TYPE_USB_CONTROLLER"
  }
}

resource "zedcloud_model" "os_EM321-102_TYPE-S-4TB" {
  name        = "os_EM321-102_TYPE-S-4TB"
  brand_id    = zedcloud_brand.operational-services.id
  description = "os_em321-102_TYPE-S-4TB-NVMe-WLAN-WWAN-SFPNIC"
  title       = "os_EM321-102_TYPE-S-4TB"

  attr = {
    Cpus     = "16"
    hsm      = "1"
    leds     = "0"
    memory   = "30G"
    storage  = "3584G"
    watchdog = "true"
  }

  origin_type    = "ORIGIN_LOCAL"
  product_status = "production"
  product_url    = ""
  state          = "SYS_MODEL_STATE_ACTIVE"
  type           = "AMD64"

  io_member_list {
    assigngrp       = "USB"
    cbattr          = {}
    cost            = 0
    logicallabel    = "USB"
    parentassigngrp = ""
    phyaddrs = {
      PciLong = "0000:00:14.0"
    }
    phylabel     = "USB"
    usage        = "ADAPTER_USAGE_UNSPECIFIED"
    usage_policy = {}
    ztype        = "IO_TYPE_USB"
  }

  io_member_list {
    assigngrp       = "COM1"
    cbattr          = {}
    cost            = 0
    logicallabel    = "COM1"
    parentassigngrp = ""
    phyaddrs = {
      Ioports = "3f8-3ff"
      Irq     = "4"
      Serial  = "/dev/ttyS0"
    }
    phylabel     = "COM1"
    usage        = "ADAPTER_USAGE_UNSPECIFIED"
    usage_policy = {}
    ztype        = "IO_TYPE_COM"
  }

  io_member_list {
    assigngrp       = "COM2"
    cbattr          = {}
    cost            = 0
    logicallabel    = "COM2"
    parentassigngrp = ""
    phyaddrs = {
      Ioports = "2f8-2ff"
      Irq     = "3"
      Serial  = "/dev/ttyS1"
    }
    phylabel     = "COM2"
    usage        = "ADAPTER_USAGE_UNSPECIFIED"
    usage_policy = {}
    ztype        = "IO_TYPE_COM"
  }

  io_member_list {
    assigngrp       = "group24"
    cbattr          = {}
    cost            = 0
    logicallabel    = "eth0"
    parentassigngrp = ""
    phyaddrs = {
      Ifname  = "eth0"
      PciLong = "0000:01:00.0"
    }
    phylabel     = "eth0"
    usage        = "ADAPTER_USAGE_MANAGEMENT"
    usage_policy = {}
    ztype        = "IO_TYPE_ETH"
  }

  io_member_list {
    assigngrp       = "group25"
    cbattr          = {}
    cost            = 0
    logicallabel    = "eth1"
    parentassigngrp = ""
    phyaddrs = {
      Ifname  = "eth1"
      PciLong = "0000:01:00.1"
    }
    phylabel     = "eth1"
    usage        = "ADAPTER_USAGE_MANAGEMENT"
    usage_policy = {}
    ztype        = "IO_TYPE_ETH"
  }

  io_member_list {
    assigngrp       = "group26"
    cbattr          = {}
    cost            = 0
    logicallabel    = "eth2"
    parentassigngrp = ""
    phyaddrs = {
      Ifname  = "eth2"
      PciLong = "0000:01:00.2"
    }
    phylabel     = "eth2"
    usage        = "ADAPTER_USAGE_MANAGEMENT"
    usage_policy = {}
    ztype        = "IO_TYPE_ETH"
  }

  io_member_list {
    assigngrp       = "group27"
    cbattr          = {}
    cost            = 0
    logicallabel    = "eth3"
    parentassigngrp = ""
    phyaddrs = {
      Ifname  = "eth3"
      PciLong = "0000:01:00.3"
    }
    phylabel     = "eth3"
    usage        = "ADAPTER_USAGE_MANAGEMENT"
    usage_policy = {}
    ztype        = "IO_TYPE_ETH"
  }

  io_member_list {
    assigngrp       = "group28"
    cbattr          = {}
    cost            = 0
    logicallabel    = "eth4"
    parentassigngrp = ""
    phyaddrs = {
      Ifname  = "eth4"
      PciLong = "0000:02:00.0"
    }
    phylabel     = "eth4"
    usage        = "ADAPTER_USAGE_MANAGEMENT"
    usage_policy = {}
    ztype        = "IO_TYPE_ETH"
  }

  io_member_list {
    assigngrp       = "group71"
    cbattr          = {}
    cost            = 0
    logicallabel    = "eth5"
    parentassigngrp = ""
    phyaddrs = {
      Ifname  = "eth5"
      PciLong = "0000:65:00.0"
    }
    phylabel     = "eth5"
    usage        = "ADAPTER_USAGE_MANAGEMENT"
    usage_policy = {}
    ztype        = "IO_TYPE_ETH"
  }

  io_member_list {
    assigngrp       = "group72"
    cbattr          = {}
    cost            = 0
    logicallabel    = "eth6"
    parentassigngrp = ""
    phyaddrs = {
      Ifname  = "eth6"
      PciLong = "0000:65:00.1"
    }
    phylabel     = "eth6"
    usage        = "ADAPTER_USAGE_MANAGEMENT"
    usage_policy = {}
    ztype        = "IO_TYPE_ETH"
  }

  io_member_list {
    assigngrp       = "VGA"
    cbattr          = {}
    cost            = 0
    logicallabel    = "VGA"
    parentassigngrp = ""
    phyaddrs = {
      PciLong = "0000:05:00.0"
    }
    phylabel     = "VGA"
    usage        = "ADAPTER_USAGE_UNSPECIFIED"
    usage_policy = {}
    ztype        = "IO_TYPE_HDMI"
  }

  io_member_list {
    assigngrp       = "group29"
    cbattr          = {}
    cost            = 0
    logicallabel    = "wlan0"
    parentassigngrp = ""
    phyaddrs = {
      Ifname  = "wlan0"
      PciLong = "0000:03:00.0"
    }
    phylabel     = "wlan0"
    usage        = "ADAPTER_USAGE_MANAGEMENT"
    usage_policy = {}
    ztype        = "IO_TYPE_WLAN"
  }

  io_member_list {
    assigngrp       = "USB"
    cbattr          = {}
    cost            = 10
    logicallabel    = "wwan0"
    parentassigngrp = ""
    phyaddrs = {
      Ifname = "wwan0"
    }
    phylabel     = "wwan0"
    usage        = "ADAPTER_USAGE_MANAGEMENT"
    usage_policy = {}
    ztype        = "IO_TYPE_WWAN"
  }
}
