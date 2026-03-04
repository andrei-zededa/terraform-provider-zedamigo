resource "zedcloud_edgenode" "this" {
  name           = var.name
  title          = coalesce(var.title, var.name)
  serialno       = var.serialno
  onboarding_key = var.onboarding_key
  model_id       = var.model_id
  project_id     = var.project_id
  admin_state    = "ADMIN_STATE_ACTIVE"


  dynamic "config_item" {
    for_each = var.ssh_pub_key != "" ? [var.ssh_pub_key] : []
    content {
      key          = "debug.enable.ssh"
      string_value = config_item.value
      # Need to set this otherwise we keep getting diff with the info in Zedcloud.
      uint64_value = "0"
    }
  }

  dynamic "interfaces" {
    for_each = var.interfaces
    content {
      intfname   = interfaces.value.intfname
      intf_usage = interfaces.value.intf_usage
      cost       = interfaces.value.cost
      netname    = interfaces.value.netname
      ztype      = interfaces.value.ztype
      tags       = interfaces.value.tags
    }
  }

  dynamic "vlan_adapters" {
    for_each = var.vlan_adapters
    content {
      logical_label    = vlan_adapters.value.logical_label
      lower_layer_name = vlan_adapters.value.lower_layer_name
      vlan_id          = vlan_adapters.value.vlan_id

      interface {
        allow_local_modifications = vlan_adapters.value.interface.allow_local_modifications
        cost                      = vlan_adapters.value.interface.cost
        intf_usage                = vlan_adapters.value.interface.intf_usage
        intfname                  = vlan_adapters.value.interface.intfname

        tags = vlan_adapters.value.interface.tags
      }
    }
  }

  tags = var.tags
}
