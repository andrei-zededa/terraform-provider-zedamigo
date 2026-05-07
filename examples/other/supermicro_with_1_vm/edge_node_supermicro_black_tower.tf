variable "onboarding_key" {
  description = "Zedcloud onboarding key"
  type        = string
  default     = "5d0767ee-0547-4569-b530-387e526f8cb9"
}

resource "zedcloud_network" "edge_node_as_dhcp_client" {
  name  = "edge_node_as_dhcp_client_${var.config_suffix}"
  title = "edge_node_as_dhcp_client"
  kind  = "NETWORK_KIND_V4"

  project_id = zedcloud_project.PROJECT.id

  ip {
    dhcp = "NETWORK_DHCP_TYPE_CLIENT"
  }
  mtu = 1500
}

resource "zedcloud_edgenode" "SUPERMICRO_BLACK_TOWER" {
  name           = "SUPERMICRO_BLACK_TOWER_${var.config_suffix}"
  title          = "SUPERMICRO_BLACK_TOWER"
  serialno       = "0123456789"
  onboarding_key = var.onboarding_key
  model_id       = zedcloud_model.SUPERMICRO_BLACK_TOWER.id
  project_id     = zedcloud_project.PROJECT.id
  admin_state    = "ADMIN_STATE_ACTIVE"

  config_item {
    key          = "debug.enable.ssh"
    string_value = var.edge_node_ssh_pub_key
    # Need to set this otherwise we keep getting diff with the info in Zedcloud.
    uint64_value = "0"
  }

  config_item {
    key          = "debug.enable.vga"
    string_value = "true"
    # Need to set this otherwise we keep getting diff with the info in Zedcloud.
    uint64_value = "0"
  }


  interfaces {
    intfname   = "eth0"
    intf_usage = "ADAPTER_USAGE_APP_SHARED"
    cost       = 0
    netname    = zedcloud_network.edge_node_as_dhcp_client.name
    net_dhcp   = "NETWORK_DHCP_TYPE_CLIENT"
    ztype      = "IO_TYPE_ETH"
    tags       = {}
  }

  interfaces {
    intfname   = "eth1"
    intf_usage = "ADAPTER_USAGE_APP_SHARED"
    cost       = 0
    netname    = zedcloud_network.edge_node_as_dhcp_client.name
    net_dhcp   = "NETWORK_DHCP_TYPE_CLIENT"
    ztype      = "IO_TYPE_ETH"
    tags       = {}
  }

  interfaces {
    intfname   = "eth2"
    intf_usage = "ADAPTER_USAGE_MANAGEMENT"
    cost       = 0
    netname    = zedcloud_network.edge_node_as_dhcp_client.name
    net_dhcp   = "NETWORK_DHCP_TYPE_CLIENT"
    ztype      = "IO_TYPE_ETH"
    tags       = {}
  }

  interfaces {
    intfname   = "eth3"
    intf_usage = "ADAPTER_USAGE_APP_SHARED"
    cost       = 0
    netname    = zedcloud_network.edge_node_as_dhcp_client.name
    net_dhcp   = "NETWORK_DHCP_TYPE_CLIENT"
    ztype      = "IO_TYPE_ETH"
    tags       = {}
  }

  tags = {}
}
