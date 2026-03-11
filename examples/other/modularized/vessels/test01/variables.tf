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

variable "management_network" {
  description = "Optional per-vessel management network with static IP configuration"
  type = object({
    name  = string
    title = optional(string, "")
    kind  = optional(string, "NETWORK_KIND_V4_ONLY")
    mtu   = optional(number, 0)
    ip = object({
      dhcp    = optional(string, "NETWORK_DHCP_TYPE_STATIC")
      subnet  = string
      gateway = string
      dns     = optional(list(string), [])
    })
  })
  default = {
    name = "no_vessel_test01"
    ip = {
      subnet  = "192.168.123.0/24"
      gateway = "192.168.123.254"
      dns     = ["9.9.9.9"]
    }
  }
}

variable "nodes" {
  description = "Map of edge nodes to create"
  type = map(object({
    model_name     = string
    serialno       = string
    onboarding_key = optional(string, "")
    ssh_pub_key    = optional(string, "")
    tags           = optional(map(string), {})
    vlans          = optional(map(list(number)), {})
    apps = optional(map(object({
      cloud_init_vars = optional(map(string), {})
      drive_images    = optional(map(string), {})
    })), {})
    interface_networks = optional(map(object({
      netname = string
      ipaddr  = optional(string, "")
    })), {})
  }))

  default = {
    "DDDD" = {
      model_name  = "os_EM321-102_TYPE-M-8TB"
      serialno    = "SN_DDDD"
      ssh_pub_key = "ssh-ed25519 AAAA.... user@example.net"
      apps = {
        "ubuntu_vm" = {
          cloud_init_vars = {
            "USERNAME"    = "labtest"
            "SSH_PUB_KEY" = "ssh-ed25519 AAAA.... user@example.net"
          }
          drive_images = {
            # For the 3rd drive of the app instance (indexes start at 0) use
            # this image name for the content-tree volume-instance. For any
            # other drives not listed here an empty block-storage volume-instance
            # will be created.
            "2" = "ubuntu_24_04_server_cloud"
          }
        }
      }
      interface_networks = {
        eth1 = { netname = "no_vessel_test01", ipaddr = "192.168.123.66" }
      }
    }
    "EEEE" = {
      model_name = "os_EM321-102_TYPE-M-8TB"
      serialno   = "SN_EEEE"
      apps       = { "ubuntu_vm" = {} }
      vlans = {
        eth1 = [2001, 2002]
      }
      interface_networks = {
        eth0 = { netname = "default_network_dhcp_client" }
      }
    }
  }
}
