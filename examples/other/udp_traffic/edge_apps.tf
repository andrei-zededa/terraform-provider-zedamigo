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
            app_port = 1700
          }
        }
        matches {
          type  = "protocol"
          value = "udp"
        }
        matches {
          type  = "lport"
          value = "1700"
        }
        matches {
          type  = "ip"
          value = "0.0.0.0/0"
        }
      }
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
