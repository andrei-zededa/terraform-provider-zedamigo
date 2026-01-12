# ssh_pub_key: If non-empty will trigger enabling SSH access to edge-nodes via
# `config_item` `debug.enable.ssh`, and to other VMs via cloud-init config.
#
# See: https://github.com/lf-edge/eve/blob/master/docs/CONFIG-PROPERTIES.md ,
# https://help.zededa.com/hc/en-us/articles/17918434708763-How-to-enable-and-disable-SSH-for-an-Edge-Device#h_01H9HCZX6K77DR2CVNC1AFJMYG .
#
# The corresponding `config_item` entry can be added both at the project level
# and per-edge-node. If both are set then the per-edge-node item will take
# precedence.
variable "ssh_pub_key" {
  description = "Enable edge-node or VM SSH access with the provided SSH public key"
  sensitive   = true
  type        = string
  default     = "ssh-ed25519 Wrong invalid@example.net"
}

# Objects in Zedcloud need to have unique names. This variable can be used to
# ensure that.
variable "config_suffix" {
  type    = string
  default = "abc123"
}

variable "user" {
  type    = string
  default = "lab"
}

# NOTE: Must download the image set the path accordingly. The path must be
# absolute.
variable "disk_image_base" {
  description = "Disk image base for creating the VMs, tested with debian-12-genericcloud-amd64.qcow2 (https://cloud.debian.org/images/cloud/bookworm/latest/debian-12-genericcloud-amd64.qcow2)"
  type        = string
  default     = "debian-12-genericcloud-amd64.qcow2"
}

locals {
  nodes = {
    "ENODE_TEST_AAAA" = "AAAA"
  }

  node_keys = sort(keys(local.nodes))

  edge_sync = {
    hostname   = "edge-sync"
    domainname = "example.net"
    mac        = "06:07:0d:00:78:e0"
    ipv4_pref  = "10.99.101.100/24"
    ipv4_addr  = "10.99.101.100" #### split("/", local.edge_sync.ipv4_pref[0])
  }
}
