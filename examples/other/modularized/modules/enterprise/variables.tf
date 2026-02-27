variable "name_suffix" {
  description = "Suffix for ensuring unique object names within a single Zedcloud enterprise"
  type        = string
}

variable "project_name" {
  description = "Name for the enterprise project"
  type        = string
  default     = "PROJECT_DEFAULT"
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

locals {
  us_name_suffix = var.name_suffix == "" ? "" : "_${var.name_suffix}"
}
