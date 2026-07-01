terraform {
  required_providers {
    zedamigo = {
      source  = "localhost/andrei-zededa/zedamigo"
      version = ">= 0.9.9"
    }

    zedcloud = {
      source  = "zededa/zedcloud"
      version = ">= 2.7.0"
    }
  }
}

provider "zedamigo" {
  # target = ""

  # Host networking (TAP, bridges) needs root privileges.
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
