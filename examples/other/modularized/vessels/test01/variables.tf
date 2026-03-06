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

# Created by the "enterprise-global" terraform config, will be referenced as a
# datasoource.
variable "enterprise_project_name" {
  description = "Name of the enterprise project to look up"
  type        = string
  default     = "Default-Project"
}

# Created by the "enterprise-global" terraform config, will be referenced as a
# datasoource. This will be used by default as the network configuration for
# an edge-node interface unless it is overridden by the `interface_networks`
# map in the `nodes` configuration.
variable "network_name" {
  description = "Name of the enterprise default network to look up"
  type        = string
  default     = "default_network_dhcp_client"
}

variable "vessel_project_name" {
  description = "Name of the project that will be created for this vessel"
  type        = string
  default     = "vessel_test01"
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
    apps               = optional(map(map(string)), {})
    interface_networks = optional(map(string), {})
  }))

  default = {
    "DDDD" = {
      model_name = "os_EM321-102_TYPE-M-8TB"
      serialno   = "SN_DDDD"
      apps       = { "ubuntu_vm" = {} }
    }
    "EEEE" = {
      model_name = "os_EM321-102_TYPE-M-8TB"
      serialno   = "SN_EEEE"
      apps = { "ubuntu_vm" = {
        "USERNAME"    = "labuser"
        "SSH_PUB_KEY" = "ssh-ed25519 AAAA..."
      } }
      vlans = {
        eth1 = [2001, 2002]
      }
      interface_networks = {
        eth0 = "default_network_dhcp_client"
      }
    }
  }
}
