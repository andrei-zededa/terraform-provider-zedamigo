resource "zedcloud_project" "PROJECT" {
  name        = "PROJECT_TEST_${var.config_suffix}"
  title       = "PROJECT_TEST_${var.config_suffix}"
  description = <<-EOF
   A test project.
  EOF

  type = "TAG_TYPE_PROJECT"
  tag_level_settings {
    flow_log_transmission = "NETWORK_INSTANCE_FLOW_LOG_TRANSMISSION_UNSPECIFIED"
    interface_ordering    = "INTERFACE_ORDERING_ENABLED"
  }
}
