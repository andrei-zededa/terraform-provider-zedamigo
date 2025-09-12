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

# Objects in Zedcloud need to have unique names. This variable can be used to
# ensure that.
variable "config_suffix" {
  type    = string
  default = "1234"
}

# Defined as a variable in the Github repo.
variable "DOCKERHUB_USERNAME" {
  sensitive = false
  type      = string
  default   = "andreizededa"
}

# Defined as a variable in the Github repo.
variable "DOCKERHUB_IMAGE_NAME" {
  sensitive = false
  type      = string
  default   = "hello-zedcloud"
}

# Most likely this comes from the trigger for the GHA workflow that calls
# terraform. The corresponding `TF_VAR_DOCKERHUB_IMAGE_LATEST_TAG` environment
# variable should be set to override the `latest` default value below.
variable "DOCKERHUB_IMAGE_LATEST_TAG" {
  sensitive = false
  type      = string
  default   = "latest"
}
