terraform {
  required_providers {
    zedamigo = {
      source  = "localhost/andrei-zededa/zedamigo"
      version = ">= 0.6.0, < 1.0.0"
    }

    zedcloud = {
      source = "zededa/zedcloud"
      # Actually we need the next version for edge-node interface ZTYPE support,
      # or a locally built version from commit 475d2c3 .
      version = ">= 2.5.0"
    }
  }
}

provider "zedamigo" {
  # target = ""
  use_sudo = true
}

variable "ZEDEDA_CLOUD_URL" {
  description = "ZEDEDA CLOUD URL"
  sensitive   = false
  type        = string
}

variable "ZEDEDA_CLOUD_TOKEN" {
  description = "ZEDEDA CLOUD API TOKEN"
  sensitive   = true
  type        = string
}

provider "zedcloud" {
  zedcloud_url   = var.ZEDEDA_CLOUD_URL
  zedcloud_token = var.ZEDEDA_CLOUD_TOKEN
}
