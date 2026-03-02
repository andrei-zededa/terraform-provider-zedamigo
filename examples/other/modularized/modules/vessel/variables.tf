variable "name_suffix" {
  description = "Suffix for ensuring unique object names within a single Zedcloud enterprise"
  type        = string
}

variable "enterprise_project_name" {
  description = "Name of the enterprise project to look up"
  type        = string
}

variable "network_name" {
  description = "Name of the enterprise default network to look up"
  type        = string
}

variable "app_name" {
  description = "Name of the enterprise app definition to look up"
  type        = string
}

variable "vessel_project_name" {
  description = "Name of the project that will be created for this vessel"
  type        = string
}

variable "nodes" {
  description = "Map of edge nodes to create"
  type = map(object({
    model_name     = string
    serialno       = string
    onboarding_key = optional(string, "")
    ssh_pub_key    = optional(string, "")
    tags           = optional(map(string), {})
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
  us_name_suffix = var.name_suffix == "" ? "" : "_${var.name_suffix}"

  # Derive edge-node interfaces automatically from the model's io_member_list
  node_interfaces = {
    for node_key, node in var.nodes : node_key => [
      for io in data.zedcloud_model.enterprise[node.model_name].io_member_list : {
        intfname   = io.logicallabel
        intf_usage = io.usage
        cost       = io.cost
        netname    = ""
        ztype      = io.ztype
        tags       = {}
      }
    ]
  }
}
