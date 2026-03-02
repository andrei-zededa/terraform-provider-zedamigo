variable "name" {
  description = "Project name"
  type        = string
}

variable "title" {
  description = "Project title (defaults to name if left empty)"
  type        = string
  default     = ""
}

variable "description" {
  description = "Project description (defaults to title if left empty)"
  type        = string
  default     = ""
}
