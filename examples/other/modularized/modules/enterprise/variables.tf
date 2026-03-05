variable "project_name" {
  description = "Name for the enterprise project"
  type        = string
  default     = "Default-Project"
}

variable "ssh_pub_key" {
  description = "SSH public key for project-level SSH config"
  type        = string
  sensitive   = true
  default     = ""
}

variable "dockerhub_username" {
  description = "Docker Hub username. If set, a specific DockerHub datastore will be created."
  type        = string
  default     = ""
}

variable "app_password" {
  description = "Default password for the app custom_config cloud-init (which creates an user)"
  type        = string
  sensitive   = true
  default     = ""
}
