resource "zedamigo_cloud_init_iso" "CI_DATA_CP_01" {
  name = "CI_DATA_CP_01"
  # Read long config data from a file. We could also use the `templatefile`
  # TF function if we need to set some variable values.
  user_data = templatefile("./cloud-init/user-data.tftpl",
    {
      user             = var.user,
      user_ssh_pub_key = var.user_ssh_pub_key,
      hostname         = local.control_plane.name,
      domainname       = local.domainname,
      custom_hosts     = local.all_hosts
    }
  )
  # Multi-line string with `heredoc`.
  meta_data = <<-EOF
  instance-id: ${local.control_plane.name}
  local-hostname: ${local.control_plane.name}
  EOF
  network_config = templatefile("./cloud-init/network-config.tftpl",
    { mac = local.control_plane.mac, ipv4_addr = local.control_plane.ipv4_addr }
  )
}

resource "zedamigo_cloud_init_iso" "CI_DATA_NODES" {
  for_each = local.nodes

  name = "CI_DATA_${each.key}"
  # Read long config data from a file. We could also use the `templatefile`
  # TF function if we need to set some variable values.
  user_data = templatefile("./cloud-init/user-data.tftpl",
    {
      user             = var.user,
      user_ssh_pub_key = var.user_ssh_pub_key,
      hostname         = each.value.name,
      domainname       = local.domainname,
      custom_hosts     = local.all_hosts
    }
  )
  # Multi-line string with `heredoc`.
  meta_data = <<-EOF
  instance-id: ${each.value.name}
  local-hostname: ${each.value.name}
  EOF
  network_config = templatefile("./cloud-init/network-config.tftpl",
    { mac = each.value.mac, ipv4_addr = each.value.ipv4_addr }
  )
}
