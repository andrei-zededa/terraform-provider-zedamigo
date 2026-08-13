#### A single local (i.e. NAT-ed) network-instance on the `uplink` port, shared
#### by all 27 edge-app-instances. Because it is the `device_default` one it is
#### also what the edge-node uses for the inbound port maps of the edge-apps.
####
#### The subnet is pinned to a /16 instead of letting EVE-OS pick one: the
#### default is a /24, which would still be enough for 27 app-instances, but
#### there is no reason to be that tight when the whole point of the example is
#### to scale the number of app-instances up.
resource "zedcloud_network_instance" "NET_INSTANCE_APP_NAT" {
  name      = "ni_local_nat_${zedcloud_edgenode.ENODE_TEST.name}"
  title     = "TF created instance of ni_local_nat for ${zedcloud_edgenode.ENODE_TEST.name}"
  kind      = "NETWORK_INSTANCE_KIND_LOCAL"
  type      = "NETWORK_INSTANCE_DHCP_TYPE_V4"
  device_id = zedcloud_edgenode.ENODE_TEST.id

  port           = "uplink"
  device_default = true

  ip {
    subnet  = "10.166.0.0/16"
    gateway = "10.166.0.1"
    dns     = ["10.166.0.1"]
    dhcp_range {
      start = "10.166.0.128"
      end   = "10.166.255.254"
    }
  }

  tags = {
    ni_local_nat = "true"
  }
}

#### The dedicated volume of every workload replica. These are the volumes which
#### drive [2] of the corresponding edge-app definition references by label, and
#### they add up to the "33GB of dedicated volumes" part of the resource budget.
####
#### The persistent volumes (the other 10GB) have no resource here on purpose:
#### drive [1] of the manifest carries no volume label, so Zedcloud creates them
#### implicitly, one per edge-app-instance. They are therefore not in the
#### Terraform state and are removed by Zedcloud together with the
#### edge-app-instance.
resource "zedcloud_volume_instance" "APP_DEDICATED_DATA" {
  for_each = local.WORKLOADS

  name  = "${each.key}_data_${var.config_suffix}"
  title = "Dedicated data volume of ${each.key}_${var.config_suffix}"

  project_id = zedcloud_project.PROJECT.id
  device_id  = zedcloud_edgenode.ENODE_TEST.id

  type       = "VOLUME_INSTANCE_TYPE_BLOCKSTORAGE"
  accessmode = "VOLUME_INSTANCE_ACCESS_MODE_READWRITE"
  # `size_bytes` is in bytes, unlike the `maxsize` of a manifest drive which is
  # in kilobytes.
  size_bytes = each.value.data_mb * 1024 * 1024

  image       = ""
  multiattach = false
  cleartext   = true

  # Take the label directly from the edge-app definition, so that the two can
  # never drift apart.
  label = zedcloud_application.CONTAINER_APP_DEFS[each.key].manifest[0].images[2].volumelabel
}

#### And finally the 27 edge-app-instances themselves, all on the same edge-node.
resource "zedcloud_application_instance" "APP_INSTANCES" {
  for_each = local.WORKLOADS

  # If we don't explicitly set the dependency then the app-instances might be
  # created before the volume-instances exist on the node, and Zedcloud rejects
  # an app-instance whose drive references a volume label for which there is no
  # volume-instance on that edge-node.
  depends_on = [zedcloud_volume_instance.APP_DEDICATED_DATA]

  name      = "${each.key}_on_${zedcloud_edgenode.ENODE_TEST.name}"
  title     = "TF created instance of ${zedcloud_application.CONTAINER_APP_DEFS[each.key].name} for ${zedcloud_edgenode.ENODE_TEST.name}"
  device_id = zedcloud_edgenode.ENODE_TEST.id
  app_id    = zedcloud_application.CONTAINER_APP_DEFS[each.key].id
  app_type  = zedcloud_application.CONTAINER_APP_DEFS[each.key].manifest[0].app_type

  activate = true

  logs {
    access = true
  }

  vminfo {
    # `cpus` is the only resource which can be overridden per-instance, and it is
    # set to the same value as the edge-app definition here. `memory` is
    # read-only, it always comes from the manifest.
    cpus = each.value.cpus
    mode = zedcloud_application.CONTAINER_APP_DEFS[each.key].manifest[0].vmmode
    vnc  = false
  }

  interfaces {
    intfname    = zedcloud_application.CONTAINER_APP_DEFS[each.key].manifest[0].interfaces[0].name
    intforder   = 1
    privateip   = false
    netinstname = zedcloud_network_instance.NET_INSTANCE_APP_NAT.name
  }
}
