# edge_node_ssh_pub_key: If non-empty will trigger enabling SSH access to
# edge-nodes via `config_item` `debug.enable.ssh`."
#
# See: https://github.com/lf-edge/eve/blob/master/docs/CONFIG-PROPERTIES.md ,
# https://help.zededa.com/hc/en-us/articles/17918434708763-How-to-enable-and-disable-SSH-for-an-Edge-Device#h_01H9HCZX6K77DR2CVNC1AFJMYG .
#
# The corresponding `config_item` entry can be added both at the project level
# and per-edge-node. If both are set then the per-edge-node item will take
# precedence.
variable "edge_node_ssh_pub_key" {
  description = "Enable edge-node SSH access with the provided SSH public key"
  sensitive   = true
  type        = string
  default     = ""
}

# EDGE_NODE_ARCH is the architecture (`amd64` or `arm64`) of the edge-nodes,
# this will be used in the model but also for selecting which EVE-OS installer
# to use and can be used for edge-app-instance images as well.
variable "EDGE_NODE_ARCH" {
  type    = string
  default = "amd64"
}

# Objects in Zedcloud need to have unique names. This variable can be used to
# ensure that.
variable "config_suffix" {
  type    = string
  default = "mc27"
}

# The size of the single edge-node of this example. It has to be big enough for
# the 27 edge-app-instances of `workloads.tf` (38 vCPU, 61 GB RAM, 43 GB of
# volumes) plus whatever EVE-OS itself needs.
#
# These values are used both for the QEMU VM which runs EVE-OS and for the
# Zedcloud model, so that the model matches the actual hardware.
variable "EDGE_NODE_CPUS" {
  description = "Number of vCPUs of the edge-node VM"
  type        = number
  default     = 80
}

variable "EDGE_NODE_MEM_GB" {
  description = "Amount of RAM, in GB, of the edge-node VM"
  type        = number
  default     = 256
}

variable "EDGE_NODE_DISK_MB" {
  description = "Size, in MB, of the single disk of the edge-node VM"
  type        = number
  default     = 1000000 # ~1TB
}
