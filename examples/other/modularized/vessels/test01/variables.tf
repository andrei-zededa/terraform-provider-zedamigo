variable "ZEDEDA_CLOUD_URL" {
  description = "ZEDEDA Cloud URL"
  sensitive   = false
  type        = string
}

variable "ZEDEDA_CLOUD_TOKEN" {
  description = "ZEDEDA Cloud API Token"
  sensitive   = true
  type        = string
}

variable "name_suffix" {
  description = "Suffix for ensuring unique object names within the same Zedcloud enterprise; set it to the empty string if you don't need this."
  type        = string
}

# Created by the "enterprise-global" terraform config, will be referenced as a
# datasoource.
variable "project_name" {
  description = "Name of the enterprise project"
  type        = string
  default     = "PROJECT_DEFAULT_test7"
}

# Created by the "enterprise-global" terraform config, will be referenced as a
# datasoource.
variable "model_name" {
  description = "Name of the enterprise model"
  type        = string
  default     = "QEMU_VM_test7"
}

# Created by the "enterprise-global" terraform config, will be referenced as a
# datasoource.
variable "network_name" {
  description = "Name of the enterprise default network"
  type        = string
  default     = "default_network_dhcp_client_test7"
}

# Created by the "enterprise-global" terraform config, will be referenced as a
# datasoource.
variable "app_name" {
  description = "Name of the enterprise app definition"
  type        = string
  default     = "ubuntu_vm_test7"
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

  default = {
    "AAAA" = {
      serialno = "SN_AAAA"
      interfaces = [
        {
          intfname   = "eth0"
          intf_usage = "ADAPTER_USAGE_MANAGEMENT"
          cost       = 0
          netname    = ""
          ztype      = "IO_TYPE_ETH"
        },
        {
          intfname   = "eth1"
          intf_usage = "ADAPTER_USAGE_APP_SHARED"
          cost       = 0
          netname    = ""
          ztype      = "IO_TYPE_ETH"
        },
      ]
    }
    "BBBB" = {
      serialno = "SN_BBBB"
      interfaces = [
        {
          intfname   = "eth0"
          intf_usage = "ADAPTER_USAGE_MANAGEMENT"
          cost       = 0
          netname    = ""
          ztype      = "IO_TYPE_ETH"
        },
        {
          intfname   = "eth1"
          intf_usage = "ADAPTER_USAGE_APP_SHARED"
          cost       = 0
          netname    = ""
          ztype      = "IO_TYPE_ETH"
        },
      ]
    }
  }
}
