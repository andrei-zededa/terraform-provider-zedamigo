data "zedcloud_project" "enterprise" {
  name  = var.project_name
  title = "" # Title is a mandatory attribute but actually for a datasource it's value is retrieved from Zedcloud.
  type  = ""
}

data "zedcloud_model" "enterprise" {
  name        = var.model_name
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
