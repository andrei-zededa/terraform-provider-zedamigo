terraform {
  required_providers {
    zedcloud = {
      source  = "zededa/zedcloud"
      version = "2.7.0-RC"
    }
  }
}

provider "zedcloud" {
  zedcloud_url   = var.ZEDEDA_CLOUD_URL
  zedcloud_token = var.ZEDEDA_CLOUD_TOKEN
}

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
