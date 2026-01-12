resource "zedamigo_cloud_init_iso" "EDGE_SYNC_CI" {
  name = "EDGE_SYNC_CI"
  # Read long config data from a file. We could also use the `templatefile`
  # TF function if we need to set some variable values.
  user_data = templatefile("./cloud-init/user-data.tftpl",
    {
      user             = var.user,
      user_ssh_pub_key = var.ssh_pub_key,
      hostname         = local.edge_sync.hostname,
      domainname       = local.edge_sync.domainname,
      custom_hosts     = [],
      extra_runcmd     = templatefile("./cloud-init/install_docker_runcmd.tftpl", { user = var.user }),
    }
  )
  # Multi-line string with `heredoc`.
  meta_data = <<-EOF
  instance-id: ${local.edge_sync.hostname}
  local-hostname: ${local.edge_sync.domainname}
  EOF
  network_config = templatefile("./cloud-init/network-config.tftpl",
    { mac = local.edge_sync.mac, ipv4_addr = local.edge_sync.ipv4_pref }
  )
}

resource "zedamigo_vm" "EDGE_SYNC" {
  name               = "edge_sync_vm"
  cpus               = 2
  mem                = "2G"
  serial_no          = "1234"
  serial_port_server = true
  disk_image_base    = abspath(var.disk_image_base)
  disk_size_mb       = "100000" # ~ 100GB
  drive_if           = "virtio" # Required because the Debian `genericcloud` image doesn't have any drivers for IDE/SATA disks.

  extra_qemu_args = [
    "-drive", "file=${zedamigo_cloud_init_iso.EDGE_SYNC_CI.filename},format=raw,if=virtio", # Same comment as for `drive_if` above.
    "-nic", "tap,id=usernet1,ifname=${zedamigo_tap.TAP_EDGE_SYNC.name},mac=${local.edge_sync.mac},script=no,downscript=no,model=virtio",
  ]
}
