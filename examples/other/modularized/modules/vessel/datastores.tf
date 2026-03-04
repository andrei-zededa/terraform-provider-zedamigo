module "vessel_datastore" {
  for_each = var.vessel_datastores
  source   = "../datastore"

  name    = each.key
  ds_type = each.value.ds_type
  ds_fqdn = each.value.ds_fqdn
  ds_path = each.value.ds_path
}
