terraform {
  required_providers {
    zedamigo = {
      source  = "localhost/andrei-zededa/zedamigo"
      version = ">= 0.12.4"
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

  target = "172.16.2.221"

  ssh {
    user               = "support_lab"
    use_agent          = true
    proxy_jump         = "root@localhost:11022"
    remote_binary_path = "/home/support_lab/.terraform.d/plugins/localhost/andrei-zededa/zedamigo/0.0.0-dev.branchfixdarwinsshlinux+commitbe34e17/linux_amd64/terraform-provider-zedamigo_v0.0.0-dev.branchfixdarwinsshlinux+commitbe34e17"
  }
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
