resource "zedamigo_internet_monitor" "test" {
  destinations = [
    "https://www.google.com",
    "https://${var.ZEDEDA_CLOUD_URL}/api/v1/version",
  ]

  interval        = "20s"
  ping_count      = 5
  ping_timeout    = "5s"
  dns_timeout     = "5s"
  http_timeout    = "10s"
  doh_endpoint    = "https://1.1.1.1/dns-query"
  flush_every_n   = 1
  privileged_icmp = true
}

resource "zedamigo_monitor_system_usage" "test" {
  depends_on = [
    zedamigo_edge_node.ENODE_001,
    zedamigo_edge_node.ENODE_002,
    zedamigo_edge_node.ENODE_003
  ]

  interval      = "10s"
  flush_every_n = 6
  include_env   = "filtered"

  namespaces = [zedamigo_netns.TEST_NS.B.name, zedamigo_netns.TEST_NS.C.name]
}
