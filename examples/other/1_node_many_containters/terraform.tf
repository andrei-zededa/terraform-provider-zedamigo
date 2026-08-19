terraform {
  required_providers {
    zedamigo = {
      source  = "localhost/andrei-zededa/zedamigo"
      version = ">= 0.13.1"
    }

    zedcloud = {
      source  = "zededa/zedcloud"
      version = "2.8.0"
    }

    random = {
      source  = "hashicorp/random"
      version = ">= 3.9.0"
    }
  }
}

provider "zedamigo" {
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
