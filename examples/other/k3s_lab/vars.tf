variable "user" {
  type    = string
  default = ""
}

# NOTE: Must be loaded into an ssh-agent so that the terraform remote-exec
# provisioner can connect to the VMs. IF an agent is not already setup you
# can:
#    eval "$(ssh-agent)";
#    ssh-add ~/.ssh/id_ed25519;
#    ssh-add -L;
variable "user_ssh_pub_key" {
  description = "SSH public key"
  sensitive   = true
  type        = string
  default     = ""
}

# NOTE: Must download the image set the path accordingly. The path must be
# absolute.
variable "disk_image_base" {
  description = "Disk image base for creating the VMs, tested with debian-12-genericcloud-amd64.qcow2 (https://cloud.debian.org/images/cloud/bookworm/latest/debian-12-genericcloud-amd64.qcow2)"
  type        = string
  default     = "debian-12-genericcloud-amd64.qcow2"
}
