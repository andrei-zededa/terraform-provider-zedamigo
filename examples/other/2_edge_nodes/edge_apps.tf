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
      ignorepurge = false
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
      ignorepurge = false
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
      ignorepurge = false
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

variable "UBUNTU_CLOUD_INIT_VARS" {
  description = "List of variables for the edge-app custom config for Ubuntu cloud-init"
  type = list(object({
    name       = string
    default    = string
    required   = bool
    label      = string
    format     = string
    encode     = string
    max_length = string
    value      = string
  }))
  default = [
    {
      name       = "USERNAME"
      default    = "labuser"
      required   = false
      label      = "The user to be created inside the newly created VM (default: `labuser`)."
      format     = "VARIABLE_FORMAT_TEXT"
      encode     = "FILE_ENCODING_UNSPECIFIED"
      max_length = "200"
      value      = ""
    },
    {
      name       = "SSH_PUB_KEY"
      default    = "ssh-ed25519 AAAAinvalid invalid@example.net"
      required   = false
      label      = "An SSH public key for authenticating as the newly created user (default: `An invalid SSH public key`)."
      format     = "VARIABLE_FORMAT_TEXT"
      encode     = "FILE_ENCODING_UNSPECIFIED"
      max_length = "200"
      value      = ""
    },
  ]
}

resource "zedcloud_application" "UBUNTU_VM_DEF" {
  name  = "ubuntu_test_vm_${var.config_suffix}"
  title = "ubuntu_test_vm_${var.config_suffix}"

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

    configuration {
      # https://help.zededa.com/hc/en-us/articles/4440323189403-Custom-Configuration-Edge-Application#01JF0TNWAFAAVRY5K7PJHYYP5Z
      custom_config {
        add                  = true
        allow_storage_resize = false
        field_delimiter      = "####"
        name                 = "config01"
        override             = false
        # template needs to be base64 encoded.
        #
        # We could also use a TF template file and let terraform do some of
        # the replacements, like:
        #     template = base64encode(templatefile("${path.module}/ubuntu_cloud_init_template.tftpl",
        #      {
        #        user             = "johndoe",
        #        user_ssh_pub_key = "ssh-ed25519 AAAAinvalid invalid@example.net",
        #        hostname         = "ubuntu2404test",
        #        domainname       = "example.net",
        #        custom_hosts     = []
        #      }))
        # However that part cannot then be changed based on each instance.
        template = filebase64("${path.module}/ubuntu_cloud_init.txt")

        variable_groups {
          name     = "Default Group 1"
          required = true

          # The list of variables for this custom config is automatically
          # generated. This is needed so that we don't duplicate the list of
          # variables for the app definition and the app instance.
          dynamic "variables" {
            for_each = var.UBUNTU_CLOUD_INIT_VARS
            content {
              name       = variables.value.name
              default    = variables.value.default
              required   = variables.value.required
              label      = variables.value.label
              format     = variables.value.format
              encode     = variables.value.encode
              max_length = variables.value.max_length
            }
          }
        }
      }
    }

    images {
      cleartext   = true
      drvtype     = "HDD"
      imageformat = "QCOW2"
      imageid     = upper(var.EDGE_NODE_ARCH) == "ARM64" ? zedcloud_image.ubuntu_24_04_server_cloud_arm64.id : zedcloud_image.ubuntu_24_04_server_cloud_amd64.id
      imagename   = upper(var.EDGE_NODE_ARCH) == "ARM64" ? zedcloud_image.ubuntu_24_04_server_cloud_arm64.name : zedcloud_image.ubuntu_24_04_server_cloud_amd64.name
      maxsize     = "0"
      mountpath   = "/"
      ignorepurge = false
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
