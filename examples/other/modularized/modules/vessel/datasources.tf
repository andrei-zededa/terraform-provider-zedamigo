data "zedcloud_project" "enterprise" {
  name  = var.enterprise_project_name
  title = "" # Title is a mandatory attribute but actually for a datasource it's value is retrieved from Zedcloud.
  type  = ""
}

data "zedcloud_model" "enterprise" {
  for_each    = toset([for node in var.nodes : node.model_name])
  name        = each.value
  brand_id    = ""
  title       = ""
  type        = ""
  state       = ""
  attr        = {}
  origin_type = ""
}

data "zedcloud_network" "enterprise" {
  name  = var.network_name
  title = ""
}

data "zedcloud_application" "enterprise" {
  name  = var.app_name
  title = ""
}
