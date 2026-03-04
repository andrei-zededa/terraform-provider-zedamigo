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
