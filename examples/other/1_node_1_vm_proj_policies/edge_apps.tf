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
    {
      name       = "PASSWORD"
      default    = ""
      required   = false
      label      = "The password for the created user (default: empty, set per-instance)."
      format     = "VARIABLE_FORMAT_TEXT"
      encode     = "FILE_ENCODING_UNSPECIFIED"
      max_length = "200"
      value      = ""
    },
  ]
}

resource "random_password" "vm_password" {
  length  = 10
  special = false
}

locals {
  # An edge-app-instance which is auto-deployed from an app-policy cannot carry
  # its own `custom_config`: the app-policy has no place to put per-instance
  # cloud-init variable values, so when Zedcloud renders the cloud-init template
  # for the auto-deployed instance it substitutes each variable with the `value`
  # from the edge-app *definition*, falling back to `default` when `value` is
  # empty.
  #
  # This is the main difference to the `1_node_1_vm` example, where the same
  # overrides are applied on the `zedcloud_application_instance` resource
  # instead. The consequence is that every instance auto-deployed from this
  # policy shares the same user/SSH key/password.
  UBUNTU_CLOUD_INIT_OVERRIDES = {
    "USERNAME" = {
      value = "labuser"
    },
    "SSH_PUB_KEY" = {
      value = var.edge_node_ssh_pub_key
    },
    "PASSWORD" = {
      value = random_password.vm_password.result
    },
  }

  # Create a deep copy of the entire list of custom config variables with the
  # overrides applied.
  EDGE_APP_UBUNTU_CLOUD_INIT_VARS = [
    for xxx in var.UBUNTU_CLOUD_INIT_VARS : merge(xxx,
      # Only try to merge if there's an override for this variable.
      contains(keys(local.UBUNTU_CLOUD_INIT_OVERRIDES), xxx.name)
      ? local.UBUNTU_CLOUD_INIT_OVERRIDES[xxx.name]
      : {}
    )
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
        template = filebase64("${path.module}/ubuntu_cloud_init.txt")

        variable_groups {
          name     = "Default Group 1"
          required = true

          # Unlike in the `1_node_1_vm` example the variables here carry a
          # `value` and not just a `default`, see the comment on
          # `local.UBUNTU_CLOUD_INIT_OVERRIDES` above.
          dynamic "variables" {
            for_each = local.EDGE_APP_UBUNTU_CLOUD_INIT_VARS
            content {
              name       = variables.value.name
              default    = variables.value.default
              required   = variables.value.required
              label      = variables.value.label
              format     = variables.value.format
              encode     = variables.value.encode
              max_length = variables.value.max_length
              value      = variables.value.value
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
      # maxsize is in kilobytes, so for example "5242880" = 5GB but it can be left at "0".
      maxsize     = 0
      mountpath   = "/"
      ignorepurge = false
      preserve    = false
      readonly    = false
      target      = "Disk"
    }

    # The persistent data disk of the VM. In the `1_node_1_vm` example this
    # drive carries a `volumelabel` and a matching `zedcloud_volume_instance`
    # resource is created explicitly for every edge-node. That is not possible
    # here: the edge-app-instance is created by Zedcloud as soon as the
    # edge-node joins the project and Zedcloud *fails* the auto-deployment if a
    # drive references a volume label for which no volume-instance exists yet on
    # that edge-node.
    #
    # Instead the drive is left without a volume label, which makes Zedcloud
    # create an implicit blank block-storage volume of `maxsize` for it. Note
    # that `maxsize` is in kilobytes here whereas `size_bytes` of a
    # `zedcloud_volume_instance` is in bytes.
    images {
      cleartext   = true
      ignorepurge = true
      imageformat = "FmtUnknown"
      imageid     = ""
      imagename   = ""
      maxsize     = 20971520 #### 20GB, in kilobytes.
      # The actual mount path is decided by the VM guest OS config.
      mountpath = "/mnt/data"
      preserve  = true
      readonly  = false
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
    }

    interfaces {
      directattach = false
      name         = "app_eth2"
      optional     = false
      privateip    = false

      acls {
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
      value = "2"
    }
    resources {
      name  = "memory"
      value = "2097152.00"
    }
    resources {
      name  = "storage"
      value = "10485760.00"
    }
  }
}
