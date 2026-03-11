variable "enterprise_project_name" {
  description = "Name of the enterprise project to look up"
  type        = string
}

variable "network_name" {
  description = "Name of the enterprise default network to look up"
  type        = string
}

variable "vessel_project_name" {
  description = "Name of the project that will be created for this vessel"
  type        = string
}

variable "management_network" {
  description = "Optional per-vessel management network with static IP configuration"
  type = object({
    name    = string
    title   = optional(string, "")
    kind    = optional(string, "NETWORK_KIND_V4_ONLY")
    mtu     = optional(number, 0)
    ip = object({
      dhcp    = optional(string, "NETWORK_DHCP_TYPE_STATIC")
      subnet  = string
      gateway = string
      dns     = optional(list(string), [])
    })
  })
  default = null
}

variable "nodes" {
  description = "Map of edge nodes to create"
  type = map(object({
    model_name         = string
    serialno           = string
    onboarding_key     = optional(string, "")
    ssh_pub_key        = optional(string, "")
    tags               = optional(map(string), {})
    vlans              = optional(map(list(number)), {})
    apps = optional(map(object({
      cloud_init_vars = optional(map(string), {})
      drive_images    = optional(map(string), {})
    })), {})
    interface_networks = optional(map(object({
      netname = string
      ipaddr  = optional(string, "")
    })), {})
  }))
}

variable "vessel_datastores" {
  description = "Optional vessel-specific datastores"
  type = map(object({
    ds_type = string
    ds_fqdn = string
    ds_path = optional(string, "")
  }))
  default = {}
}

locals {
  # Derive edge-node interfaces automatically from the model's io_member_list
  node_interfaces = {
    for node_key, node in var.nodes : node_key => [
      for io in data.zedcloud_model.enterprise[node.model_name].io_member_list : {
        intfname   = io.logicallabel
        intf_usage = io.usage
        cost       = io.cost
        netname    = contains(keys(node.interface_networks), io.logicallabel) ? (
          var.management_network != null && node.interface_networks[io.logicallabel].netname == var.management_network.name
            ? zedcloud_network.management[0].name
            : data.zedcloud_network.interface_net[node.interface_networks[io.logicallabel].netname].name
        ) : data.zedcloud_network.enterprise.name
        ipaddr     = contains(keys(node.interface_networks), io.logicallabel) ? node.interface_networks[io.logicallabel].ipaddr : ""
        ztype      = io.ztype
        tags       = {}
      }
    ]
  }

  # Flatten per-node VLAN definitions into vlan_adapters lists
  node_vlan_adapters = {
    for node_key, node in var.nodes : node_key => flatten([
      for intf, vlan_ids in node.vlans : [
        for vlan_id in vlan_ids : {
          logical_label    = "${intf}.v${vlan_id}"
          lower_layer_name = intf
          vlan_id          = vlan_id
          interface = {
            intfname                  = "${intf}.v${vlan_id}"
            intf_usage                = "ADAPTER_USAGE_APP_SHARED"
            cost                      = 0
            allow_local_modifications = false
            tags                      = {}
          }
        }
      ]
    ])
  }
}
