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
  default = "abc1"
}

variable "DOCKERHUB_USERNAME" {
  sensitive = false
  type      = string
  default   = "andreizededa"
}

variable "DOCKERHUB_IMAGE_NAME" {
  sensitive = false
  type      = string
  default   = "hello-zedcloud"
}

variable "DOCKERHUB_IMAGE_LATEST_TAG" {
  sensitive = false
  type      = string
  default   = "v0.8.5"
}

variable "HELLO_ZEDCLOUD_APP_USERNAME" {
  sensitive = false
  type      = string
  default   = "user1"
}

variable "HELLO_ZEDCLOUD_APP_PASSWORD" {
  sensitive = true
  type      = string
  default   = "pass1"
}
