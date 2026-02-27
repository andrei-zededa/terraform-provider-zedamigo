resource "zedcloud_datastore" "this" {
  name        = var.name
  title       = coalesce(var.title, var.name)
  ds_type     = var.ds_type
  ds_fqdn     = var.ds_fqdn
  ds_path     = var.ds_path
  description = var.description
}
