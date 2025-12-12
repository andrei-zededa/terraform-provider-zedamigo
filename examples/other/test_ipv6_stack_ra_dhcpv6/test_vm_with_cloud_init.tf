locals {
  vm_name = "test-vm-01"
}

resource "zedamigo_cloud_init_iso" "CI_DATA_01" {
  name = "CI_DATA_01"
  # Read long config data from a file. We could also use the `templatefile`
  # TF function if we need to set some variable values.
  user_data = templatefile("./cloud-init/user-data.tftpl",
    {
      user             = var.username,
      user_ssh_pub_key = var.user_ssh_pub_key,
      hostname         = local.vm_name,
      domainname       = "test-ipv6.example.net",
      custom_hosts     = [],
    }
  )
  # Multi-line string with `heredoc`.
  meta_data      = <<-EOF
  instance-id: ${local.vm_name}.test-ipv6.example.net 
  local-hostname: ${local.vm_name} 
  EOF
  network_config = file("./cloud-init/network-config")
}

resource "zedamigo_edge_node" "TEST_VM_01" {
  name               = local.vm_name
  cpus               = 4
  mem                = "4G"
  serial_no          = "1000"
  serial_port_server = true
  disk_image_base    = "/home/ev-zed1/ZED/testing/images_datastore/noble-server-cloudimg-amd64.daily.20250313.img"
  disk_size_mb       = "100000" # ~ 100GB

  extra_qemu_args = [
    "-drive", "file=${zedamigo_cloud_init_iso.CI_DATA_01.filename},format=raw,if=virtio",
    "-nic", "tap,id=vmnet1,ifname=${zedamigo_tap.TAP_01.name},script=no,downscript=no,model=e1000,mac=8c:84:74:11:01:01",
  ]
}
