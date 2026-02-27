variable "name" {
  description = "Edge node name"
  type        = string
}

variable "model_id" {
  description = "Model ID for the edge node"
  type        = string
}

variable "project_id" {
  description = "Project ID for the edge node"
  type        = string
}

variable "serialno" {
  description = "Serial number for the edge node"
  type        = string
}

variable "onboarding_key" {
  description = "Onboarding key for the edge node"
  type        = string
  default     = "5d0767ee-0547-4569-b530-387e526f8cb9"
}

variable "title" {
  description = "Edge node title (defaults to name)"
  type        = string
  default     = ""
}

variable "ssh_pub_key" {
  description = "SSH public key for debug.enable.ssh config item"
  type        = string
  sensitive   = true
  default     = ""
}

variable "tags" {
  description = "Tags for the edge node"
  type        = map(string)
  default     = {}
}

variable "interfaces" {
  description = "List of network interfaces for the edge node"
  type = list(object({
    intfname   = string
    intf_usage = string
    cost       = number
    netname    = string
    ztype      = string
    tags       = optional(map(string), {})
  }))
}
