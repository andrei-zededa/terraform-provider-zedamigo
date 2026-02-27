variable "ZEDEDA_CLOUD_URL" {
  description = "ZEDEDA Cloud URL"
  sensitive   = false
  type        = string
}

variable "ZEDEDA_CLOUD_TOKEN" {
  description = "ZEDEDA Cloud API Token"
  sensitive   = true
  type        = string
}

variable "name_suffix" {
  description = "Suffix for ensuring unique object names within the same Zedcloud enterprise; set it to the empty string if you don't need this."
  type        = string
}
