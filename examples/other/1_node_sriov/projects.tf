resource "zedcloud_project" "PROJECT" {
  name        = "PROJECT_TEST_${var.config_suffix}"
  title       = "PROJECT_TEST_${var.config_suffix}"
  description = <<-EOF
   A test project.
  EOF

  edgeview_policy {
    name  = "PROJECT_TEST_${var.config_suffix}.edgeviewPolicy"
    title = "EDGE_VIEW_POL_01"
    type  = "POLICY_TYPE_EDGEVIEW"

    edgeview_policy {
      max_expire_sec = 604800
      max_inst       = 3
      edgeview_allow = true

      edgeviewcfg {
        ext_policy {
          allow_ext = true
        }
        app_policy {
          allow_app = true
        }
        dev_policy {
          allow_dev = true
        }

        jwt_info {
          allow_sec  = 18000
          disp_url   = "${var.ZEDEDA_CLOUD_URL}/api/v1/edge-view"
          encrypt    = false
          expire_sec = "0"
          num_inst   = 1
        }
      }
    }
  }

  type = "TAG_TYPE_PROJECT"
  tag_level_settings {
    flow_log_transmission = "NETWORK_INSTANCE_FLOW_LOG_TRANSMISSION_UNSPECIFIED"
    interface_ordering    = "INTERFACE_ORDERING_ENABLED"
  }
}
