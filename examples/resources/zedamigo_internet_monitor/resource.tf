resource "zedamigo_internet_monitor" "test" {
  destinations = [
    "https://www.google.com",
    "https://www.cloudflare.com",
    "https://www.zededa.com",
  ]

  interval     = "60s"
  ping_count   = 5
  ping_timeout = "5s"
  dns_timeout  = "5s"
  http_timeout = "10s"
  #### doh_endpoint    = "https://dns.quad9.net/dns-query"
  doh_endpoint    = "https://9.9.9.9/dns-query"
  flush_every_n   = 1
  privileged_icmp = false

  # output_file is optional — defaults to <resource_dir>/output.msu.cbor.
  # output_file = "/var/log/internet-monitor.msu.cbor"
}
