terraform {
  required_providers {
    zedcloud = {
      source  = "zededa/zedcloud"
      version = ">= 2.6.0"
    }
  }

  backend "http" {
    address        = "http://192.168.192.168:9000/vessels/test01"
    lock_address   = "http://192.168.192.168:9000/vessels/test01"
    unlock_address = "http://192.168.192.168:9000/vessels/test01"
    username       = "basic"
    password       = "some-random-secret"
  }
}

provider "zedcloud" {
  zedcloud_url   = var.ZEDEDA_CLOUD_URL
  zedcloud_token = var.ZEDEDA_CLOUD_TOKEN
}
