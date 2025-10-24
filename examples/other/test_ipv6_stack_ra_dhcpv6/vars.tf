variable "username" {
  description = "User to be created in VMs/nodes"
  type        = string
  default     = "lab"
}

variable "user_ssh_pub_key" {
  description = "Enable SSH access with the provided SSH public key"
  sensitive   = true
  type        = string
  default     = "ssh-ed25519 Invalid nobody@example.net"
}
