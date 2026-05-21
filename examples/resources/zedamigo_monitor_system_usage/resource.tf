resource "zedamigo_monitor_system_usage" "test" {
  interval      = "10s"
  flush_every_n = 6
  include_env   = "filtered"

  # Optional: additionally monitor these network namespaces (msu-collect -n).
  # namespaces = ["ns1", "ns2"]

  # output_file is optional — defaults to <resource_dir>/data.msu.cbor.
  # output_file = "/var/log/monitor-system-usage.msu.cbor"

  # netns is optional — when set, the daemon RUNS inside that namespace
  # (orthogonal to `namespaces` above which selects what to MONITOR).
  # netns = "vmnet"
}
