module "vessel_datastore" {
  for_each = var.vessel_datastores
  source   = "../datastore"

  name    = "${each.key}_${var.name_suffix}"
  ds_type = each.value.ds_type
  ds_fqdn = each.value.ds_fqdn
  ds_path = each.value.ds_path
}
