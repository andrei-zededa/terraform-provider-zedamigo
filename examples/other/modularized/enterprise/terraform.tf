terraform {
  required_providers {
    zedcloud = {
      source  = "zededa/zedcloud"
      version = ">= 2.6.0"
    }
  }
}

provider "zedcloud" {
  zedcloud_url   = var.ZEDEDA_CLOUD_URL
  zedcloud_token = var.ZEDEDA_CLOUD_TOKEN
}
