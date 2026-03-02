resource "zedcloud_project" "this" {
  name        = var.name
  title       = coalesce(var.title, var.name)
  description = coalesce(var.description, var.title, var.name)

  type = "TAG_TYPE_PROJECT"

  tag_level_settings {
    flow_log_transmission = "NETWORK_INSTANCE_FLOW_LOG_TRANSMISSION_UNSPECIFIED"
    interface_ordering    = "INTERFACE_ORDERING_ENABLED"
  }
}
