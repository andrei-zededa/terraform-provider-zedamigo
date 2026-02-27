variable "name_suffix" {
  description = "Unique suffix for naming vessel objects"
  type        = string
}

variable "project_name" {
  description = "Name of the enterprise project to look up"
  type        = string
}

variable "model_name" {
  description = "Name of the enterprise model to look up"
  type        = string
}

variable "network_name" {
  description = "Name of the enterprise default network"
  type        = string
}

variable "app_name" {
  description = "Name of the enterprise app definition to look up"
  type        = string
}

variable "nodes" {
  description = "Map of edge nodes to create"
  type = map(object({
    serialno       = string
    onboarding_key = optional(string, "")
    ssh_pub_key    = optional(string, "")
    tags           = optional(map(string), {})
    interfaces = list(object({
      intfname   = string
      intf_usage = string
      cost       = number
      netname    = string
      ztype      = string
      tags       = optional(map(string), {})
    }))
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
