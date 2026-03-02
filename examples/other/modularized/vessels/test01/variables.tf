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
variable "enterprise_project_name" {
  description = "Name of the enterprise project to look up"
  type        = string
  default     = "default_project"
}

# Created by the "enterprise-global" terraform config, will be referenced as a
# datasoource.
variable "network_name" {
  description = "Name of the enterprise default network to look up"
  type        = string
  default     = "default_network_dhcp_client"
}

# Created by the "enterprise-global" terraform config, will be referenced as a
# datasoource.
variable "app_name" {
  description = "Name of the enterprise app definition to look up"
  type        = string
  default     = "ubuntu_vm"
}

variable "vessel_project_name" {
  description = "Name of the project that will be created for this vessel"
  type        = string
  default     = "vessel_test01"
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

  default = {
    "DDDD" = {
      model_name = "QEMU_VM_DDDD"
      serialno   = "SN_DDDD"
    }
    "EEEE" = {
      model_name = "QEMU_VM_EEEE"
      serialno   = "SN_EEEE"
    }
  }
}
