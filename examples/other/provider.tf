terraform {
  required_providers {
    zedamigo = {
      source = "andrei-zededa/zedamigo"
    }
  }
}

provider "zedamigo" {
  # target = ""
}

data "zedamigo_system_memory" "example" {}

resource "zedamigo_disk_image" "test" {
  name    = "test123"
  size_mb = 10000
}

//  resource "zedamigo_disk_image" "test_bbbb" {
//    name    = "test456"
//    size_mb = 2048
//  }
//  
//  resource "zedamigo_eve_installer" "eve1330" {
//    name    = "eve1330"
//    tag     = "13.3.0-kvm-amd64"
//    cluster = "zedcloud.gmwtus.zededa.net"
//  }
//  
//  resource "zedamigo_eve_installer" "eve1330_2nd" {
//    name    = "eve1330"
//    tag     = "13.3.0-kvm-amd64"
//    cluster = "zedcloud.hummingbird.zededa.net"
//  }

data "zedamigo_eve_installer" "eve1440" {
  filename = "/home/ev-zed1/ZED/git/github.com/andrei-zededa/zededa-cloud-speedrun/14.4.0-kvm-amd64.gmwtus_custom_installer.iso"
}

resource "zedamigo_installed_edge_node" "red01_inst" {
  name            = "red01"
  serial_no       = "0001"
  installer_iso   = data.zedamigo_eve_installer.eve1440.filename
  disk_image_base = zedamigo_disk_image.test.filename
}

resource "zedamigo_edge_node" "red01" {
  name            = "red01"
  serial_no       = "0001"
  disk_image_base = zedamigo_installed_edge_node.red01_inst.disk_image
  ovmf_vars_src   = zedamigo_installed_edge_node.red01_inst.ovmf_vars
}
