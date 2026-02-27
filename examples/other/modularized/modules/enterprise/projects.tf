resource "zedcloud_project" "this" {
  name        = "${var.project_name}${local.us_name_suffix}"
  title       = "${var.project_name}${local.us_name_suffix}"
  description = "${var.project_name}${local.us_name_suffix}"

  type = "TAG_TYPE_PROJECT"

  tag_level_settings {
    flow_log_transmission = "NETWORK_INSTANCE_FLOW_LOG_TRANSMISSION_UNSPECIFIED"
    interface_ordering    = "INTERFACE_ORDERING_ENABLED"
  }
}
