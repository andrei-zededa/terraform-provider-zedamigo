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

  tags = var.tags
}
