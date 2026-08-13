#### One edge-app definition of type container per workload replica of
#### `workloads.tf`, so 27 of them.
####
#### Having one edge-app definition *per replica* rather than one per role is
#### deliberate, for two reasons:
####
####   1. The RAM of an edge-app-instance can only come from the manifest
####      `resources`: `vminfo` of an edge-app-instance can override `cpus`, but
####      its `memory` attribute is read-only. So replicas which differ in RAM
####      need to differ in their edge-app definition.
####   2. The dedicated volume of a replica is bound to the drive via a *volume
####      label*, and Zedcloud resolves a volume label to at most one
####      volume-instance per edge-node. Since all 27 replicas land on the same
####      edge-node every replica needs its own label, and the label is part of
####      the manifest.
####
#### Each manifest declares three drives:
####
####   [0] the container image itself, mounted at `/`. Zedcloud creates an
####       implicit read-only content-tree volume for it.
####   [1] a persistent volume, with a `maxsize` but *no* volume label. Zedcloud
####       creates an implicit blank block-storage volume of exactly `maxsize`
####       for every edge-app-instance. Note that `maxsize` is in kilobytes.
####   [2] a dedicated volume, with a volume label and no `maxsize`. Zedcloud
####       resolves the label against the volume-instances of the edge-node, so
####       this one is backed by an explicit `zedcloud_volume_instance`, see
####       `edge_app_instances.tf`.
####
#### Roles whose image needs environment in order to start at all - PostgreSQL
#### refuses to initialize without `POSTGRES_PASSWORD`, and its container exiting
#### takes the whole app-instance VM down with it - carry it in a
#### `configuration { custom_config { … } }` block, generated from the `env` of
#### the role. They are configured to come up *standalone*, not to form clusters
#### with each other.
resource "zedcloud_application" "CONTAINER_APP_DEFS" {
  for_each = local.WORKLOADS

  name  = "${each.key}_${var.config_suffix}"
  title = "${each.key}_${var.config_suffix}"

  networks    = 1
  origin_type = "ORIGIN_LOCAL"

  user_defined_version = local.CONTAINER_IMAGE_TAGS[each.value.image]

  manifest {
    ac_kind             = "PodManifest"
    ac_version          = local.CONTAINER_IMAGE_TAGS[each.value.image]
    app_type            = "APP_TYPE_CONTAINER"
    cpu_pinning_enabled = false
    deployment_type     = "DEPLOYMENT_TYPE_STAND_ALONE"
    # 27 containers which nobody is going to look at over VNC.
    #
    # Note that this does *not* stop Zedcloud from allocating a VNC display
    # number for each of the resulting app-instances out of a per-edge-node pool
    # of 63 - that allocation is unconditional. See the caveat about
    # "cannot allocate VNC display number for instance" in the README.
    enablevnc = false
    name      = each.key
    vmmode    = "HV_PV"

    desc {
      agreement_list  = {}
      app_category    = "APP_CATEGORY_UNSPECIFIED"
      category        = "APP_CATEGORY_DEVOPS"
      license_list    = {}
      logo            = {}
      screenshot_list = {}
    }

    # The container environment, for the roles which need one. See
    # `local.WORKLOAD_CLOUD_INIT` in `workloads.tf` for how a cloud-config
    # `runcmd` list becomes the environment of a container.
    #
    # Unlike `1_node_1_container` this uses no Zedcloud custom-config
    # *variables*: Terraform renders the final user-data, so there is nothing
    # left for Zedcloud to substitute. Consequently there is also no
    # `custom_config` block on the edge-app-*instances* - with `override = false`
    # the template is taken from the edge-app definition, and variable groups on
    # an instance only exist to supply values for substitution.
    #
    # `field_delimiter` is left empty for the same reason. It also disables
    # Zedcloud's "the custom config still has variables unsubstituted" check,
    # which would otherwise trip over any literal `###` in the user-data.
    dynamic "configuration" {
      for_each = length(each.value.env) > 0 ? [1] : []

      content {
        custom_config {
          add                  = true
          allow_storage_resize = false
          field_delimiter      = ""
          name                 = "config01"
          override             = false
          template             = base64encode(local.WORKLOAD_CLOUD_INIT[each.key])
        }
      }
    }

    # Drive [0]: the container image.
    images {
      cleartext   = false
      ignorepurge = false
      imageformat = "CONTAINER"
      imageid     = local.CONTAINER_IMAGES[each.value.image].id
      imagename   = local.CONTAINER_IMAGES[each.value.image].name
      maxsize     = "0"
      mountpath   = "/"
      preserve    = false
      readonly    = false
    }

    # Drive [1]: the persistent volume. No volume label and no image, which is
    # what makes Zedcloud create an implicit blank block-storage volume of
    # `maxsize` for it. `maxsize` is in kilobytes, `persist_mb` is in megabytes.
    images {
      cleartext   = true
      ignorepurge = true
      imageformat = "FmtUnknown"
      imageid     = ""
      imagename   = ""
      maxsize     = tostring(each.value.persist_mb * 1024)
      mountpath   = each.value.persist_path
      preserve    = true
      readonly    = false
    }

    # Drive [2]: the dedicated volume. The size comes from the matching
    # `zedcloud_volume_instance`, not from `maxsize`.
    images {
      cleartext   = true
      ignorepurge = true
      imageformat = "FmtUnknown"
      imageid     = ""
      imagename   = ""
      maxsize     = "0"
      mountpath   = each.value.data_path
      preserve    = true
      readonly    = false
      volumelabel = "${each.key}_data_${var.config_suffix}"
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

      # Port-map the service port of the container to a per-workload port of the
      # edge-node, the equivalent of `docker run -p <node_port>:<app_port>`. See
      # the comment on `local.WORKLOAD_NODE_PORT_BASE` in `workloads.tf`.
      acls {
        actions {
          drop       = false
          limit      = false
          limitburst = 0
          limitrate  = 0
          portmap    = true

          portmapto {
            # This is the application instance port.
            app_port = each.value.app_port
          }
        }
        matches {
          type  = "protocol"
          value = "tcp"
        }
        matches {
          # This is the edge-node port.
          type  = "lport"
          value = tostring(each.value.node_port)
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
      value = "Custom"
    }

    resources {
      name  = "cpus"
      value = tostring(each.value.cpus)
    }

    # `memory` is in kilobytes.
    resources {
      name  = "memory"
      value = format("%.2f", each.value.memory_mb * 1024)
    }
  }
}
