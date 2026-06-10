variable "username" {
  description = "User to be created in the VM"
  type        = string
  default     = "lab"
}

variable "user_ssh_pub_key" {
  description = "Enable SSH access with the provided SSH public key"
  sensitive   = true
  type        = string
  default     = "ssh-ed25519 Invalid nobody@example.net"
}

# For example download an Ubuntu cloud image locally:
#   wget https://cloud-images.ubuntu.com/noble/20260518/noble-server-cloudimg-amd64.img
variable "ubuntu_image" {
  description = "Path to an Ubuntu cloud image (.img/.qcow2) used as the VM disk backing file."
  type        = string
  default     = "noble-server-cloudimg-amd64.img"
}

locals {
  final_ubuntu_image = "${path.cwd}/${var.ubuntu_image}"
}
