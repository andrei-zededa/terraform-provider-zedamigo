locals {
  # We take the container image tag and trim any leading `v` to use it
  # for the 2 edge-app version fields. Although this is not strictly
  # necessary as the 2 edge-app version fields are freeform strings.
  image_version = replace(var.DOCKERHUB_IMAGE_LATEST_TAG, "/^v/", "")
}

# This defines an edge-app of type container that can be deployed on an
# edge-node by creating a per-edge-node edge-app-instance. The instance
# can be created either specifically per-edge-node or it can be created
# automatically for every edge-node that becomes part of a project with
# an app policy, as it is done in this example.
#
# The edge-app definition uses the container image defined in `images.tf`
# and also configures the following:
#   - Resources (no. vCPUs & RAM) that will be allocated to each instance.
#   - A "custom config" that sets a couple of environment variables
#     (the end result will be the same as `docker run --env A=B`).
#   - An interface named `port_forwarding` (that name is for management
#     purposes only, doesn't actually translate to anything in the
#     running container). The interface has ACL with portmap edge-node port
#     8080 to app port 8080, this is similar to running
#     `docker run -p 8080:8080`.
resource "zedcloud_application" "CONTAINER_APP_DEF" {
  name  = "${var.DOCKERHUB_IMAGE_NAME}_container_app_${var.config_suffix}"
  title = "${var.DOCKERHUB_IMAGE_NAME}_container_app_${var.config_suffix}"

  networks    = 1
  origin_type = "ORIGIN_LOCAL"

  user_defined_version = local.image_version

  manifest {
    ac_kind             = "PodManifest"
    ac_version          = local.image_version
    app_type            = "APP_TYPE_CONTAINER"
    cpu_pinning_enabled = false
    deployment_type     = "DEPLOYMENT_TYPE_STAND_ALONE"
    enablevnc           = false
    name                = "${var.DOCKERHUB_IMAGE_NAME}_app_definition"
    vmmode              = "HV_PV"

    desc {
      agreement_list  = {}
      app_category    = "APP_CATEGORY_UNSPECIFIED"
      category        = "APP_CATEGORY_DEVOPS"
      license_list    = {}
      logo            = {}
      screenshot_list = {}
    }

    images {
      cleartext   = false
      ignorepurge = true
      imageformat = "CONTAINER"
      imageid     = zedcloud_image.CONTAINER_IMAGE.id
      imagename   = zedcloud_image.CONTAINER_IMAGE.name
      maxsize     = "0"
      mountpath   = "/"
      preserve    = false
      readonly    = false
    }

    images {
      cleartext   = false
      ignorepurge = true
      imageformat = "FmtUnknown"
      imageid     = ""
      imagename   = ""
      maxsize     = "0"
      mountpath   = "/srv/app_data"
      preserve    = false
      readonly    = false
      volumelabel = "app_data.persistence.srv"
    }

    images {
      cleartext   = false
      ignorepurge = true
      imageformat = "FmtUnknown"
      imageid     = ""
      imagename   = ""
      maxsize     = "0"
      mountpath   = "/var/lib/app_data"
      preserve    = false
      readonly    = false
      volumelabel = "app_data.persistence.var"
    }

    interfaces {
      directattach = false
      name         = "port_forwarding"
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
            # This is the application instance port.
            app_port = 8080
          }
        }
        matches {
          type  = "protocol"
          value = "tcp"
        }
        matches {
          # This is the edge-node port.
          type  = "lport"
          value = "8080"
        }
        matches {
          # Source address of the traffic.
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
            # This is the application instance port.
            app_port = 2022
          }
        }
        matches {
          type  = "protocol"
          value = "tcp"
        }
        matches {
          # This is the edge-node port.
          type  = "lport"
          value = "2022"
        }
        matches {
          # Source address of the traffic.
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
      value = "Tiny"
    }
    resources {
      name  = "cpus"
      value = "1"
    }
    resources {
      name  = "memory"
      value = "524288.00"
    }
  }
}

resource "zedcloud_application" "UBUNTU_VM_DEF" {
  name  = "ubuntu_test_vm_${var.config_suffix}"
  title = "ubuntu_test_vm_${var.config_suffix}"

  # App config including manifest created by first creating it using a manifest.json
  # (which can be exported with `zcli`) and then `terraform state show ...` on that.

  networks    = 3
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
      # imageid     = zedcloud_image.ubuntu_24_04_test.id
      # imagename   = zedcloud_image.ubuntu_24_04_test.name
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

    interfaces {
      directattach = false
      name         = "app_eth1"
      optional     = false
      privateip    = false
    }

    interfaces {
      directattach = false
      name         = "app_eth2"
      optional     = false
      privateip    = false
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
