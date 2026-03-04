resource "zedcloud_project" "this" {
  name        = var.project_name
  title       = var.project_name
  description = var.project_name

  type = "TAG_TYPE_PROJECT"

  tag_level_settings {
    flow_log_transmission = "NETWORK_INSTANCE_FLOW_LOG_TRANSMISSION_UNSPECIFIED"
    interface_ordering    = "INTERFACE_ORDERING_ENABLED"
  }
}
