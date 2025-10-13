resource "zedcloud_application" "UBUNTU_CLOUD_VM_DEF" {
  name  = "ubuntu_cloud_vm_test_${var.config_suffix}"
  title = "ubuntu_cloud_vm_test_${var.config_suffix}"

  networks    = 1
  origin_type = "ORIGIN_LOCAL"

  manifest {
    ac_kind             = "VMManifest"
    ac_version          = "1.2.0"
    app_type            = "APP_TYPE_VM"
    cpu_pinning_enabled = false
    deployment_type     = "DEPLOYMENT_TYPE_STAND_ALONE"
    enablevnc           = true
    name                = "ubuntu_test"
    vmmode              = "HV_HVM"

    images {
      cleartext   = true
      drvtype     = "HDD"
      imageformat = "QCOW2"
      imageid     = zedcloud_image.ubuntu_24_04_server_cloud.id
      imagename   = zedcloud_image.ubuntu_24_04_server_cloud.name
      maxsize     = "10485760"
      mountpath   = "/"
      ignorepurge = true
      preserve    = false
      readonly    = false
      target      = "Disk"
    }

    interfaces {
      directattach = false
      name         = "app_eth0"
      optional     = false
      privateip    = false

      acls {
        matches {
          type  = "ip"
          value = "0.0.0.0/0"
        }
      }
      acls {
        actions {
          drop       = false
          limit      = false
          limitburst = 0
          limitrate  = 0
          portmap    = true

          portmapto {
            app_port = 22
          }
        }
        matches {
          type  = "protocol"
          value = "tcp"
        }
        matches {
          type  = "lport"
          value = "10022"
        }
        matches {
          type  = "ip"
          value = "0.0.0.0/0"
        }
      }
    }

    owner {
      email   = "andrei@zededa.com"
      user    = "Andrei AT Zededa"
      website = "help.zededa.com"
    }

    resources {
      name  = "resourceType"
      value = "Custom"
    }
    resources {
      name  = "cpus"
      value = "1"
    }
    resources {
      name  = "memory"
      value = "393216.00"
    }
    resources {
      name  = "storage"
      value = "10485760.00"
    }
  }
}

resource "zedcloud_application" "UBUNTU_LOCAL_VM_DEF" {
  name  = "ubuntu_local_vm_test_${var.config_suffix}"
  title = "ubuntu_local_vm_test_${var.config_suffix}"

  networks    = 1
  origin_type = "ORIGIN_LOCAL"

  manifest {
    ac_kind             = "VMManifest"
    ac_version          = "1.2.0"
    app_type            = "APP_TYPE_VM"
    cpu_pinning_enabled = false
    deployment_type     = "DEPLOYMENT_TYPE_STAND_ALONE"
    enablevnc           = true
    name                = "ubuntu_test"
    vmmode              = "HV_HVM"

    images {
      cleartext   = true
      drvtype     = "HDD"
      imageformat = "QCOW2"
      imageid     = zedcloud_image.ubuntu_24_04_local_image.id
      imagename   = zedcloud_image.ubuntu_24_04_local_image.name
      maxsize     = "10485760"
      mountpath   = "/"
      ignorepurge = true
      preserve    = false
      readonly    = false
      target      = "Disk"
    }

    interfaces {
      directattach = false
      name         = "app_eth0"
      optional     = false
      privateip    = false

      acls {
        matches {
          type  = "ip"
          value = "0.0.0.0/0"
        }
      }
      acls {
        actions {
          drop       = false
          limit      = false
          limitburst = 0
          limitrate  = 0
          portmap    = true

          portmapto {
            app_port = 22
          }
        }
        matches {
          type  = "protocol"
          value = "tcp"
        }
        matches {
          type  = "lport"
          value = "10023"
        }
        matches {
          type  = "ip"
          value = "0.0.0.0/0"
        }
      }
    }

    owner {
      email   = "andrei@zededa.com"
      user    = "Andrei AT Zededa"
      website = "help.zededa.com"
    }

    resources {
      name  = "resourceType"
      value = "Custom"
    }
    resources {
      name  = "cpus"
      value = "1"
    }
    resources {
      name  = "memory"
      value = "393216.00"
    }
    resources {
      name  = "storage"
      value = "10485760.00"
    }
  }
}
