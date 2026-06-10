locals {
  vm_name = "test-lag-vm-01"
}

resource "zedamigo_cloud_init_iso" "CI_DATA_01" {
  name = "CI_DATA_LAG_01"

  # User-data: create the login user with the provided SSH key and install a
  # couple of tools handy for inspecting the bond (ethtool, net-tools).
  user_data = templatefile("./cloud-init/user-data.tftpl",
    {
      user             = var.username,
      user_ssh_pub_key = var.user_ssh_pub_key,
      hostname         = local.vm_name,
      domainname       = "test-lag.example.net",
    }
  )

  meta_data = <<-EOF
  instance-id: ${local.vm_name}.test-lag.example.net
  local-hostname: ${local.vm_name}
  EOF

  # Network-config: defines the VM-side bond (802.3ad / LACP fast) over the two
  # NICs below, matched by MAC address. See ./cloud-init/network-config.
  network_config = file("./cloud-init/network-config")
}

resource "zedamigo_edge_node" "TEST_VM_01" {
  name               = local.vm_name
  cpus               = 2
  mem                = "2G"
  serial_no          = "1000"
  serial_port_server = true
  disk_image_base    = local.final_ubuntu_image
  disk_size_mb       = "20000" # ~ 20GB
  drive_if           = "virtio"

  # nic0 is left at its default (QEMU user-mode networking): it provides the
  # VM's management/SSH access and outbound Internet for cloud-init. Its MAC is
  # QEMU's default 52:54:00:12:34:56 (matched as `mgmt` in the network-config).
  extra_qemu_args = [
    "-drive", "file=${zedamigo_cloud_init_iso.CI_DATA_01.filename},format=raw,if=virtio",

    # Two NICs wired to the host TAPs; inside the VM these become the bond
    # members. The MACs here MUST match the ones in cloud-init/network-config.
    # NOTE: speed and duplex are a MUST, otherwise inside of the VM these being virtio
    # NICs the guest kernel will not see a speed/duplex setting and cannot configure
    # them with LACP (other bond modes might work without speed/duplex).
    "-netdev", "tap,id=net-lagm0,ifname=${zedamigo_tap.LAG_M0.name},script=no,downscript=no",
    "-device", "virtio-net-pci,id=nic-lagm0,netdev=net-lagm0,mac=52:54:00:00:0a:01,speed=10000,duplex=full",
    "-netdev", "tap,id=net-lagm1,ifname=${zedamigo_tap.LAG_M1.name},script=no,downscript=no",
    "-device", "virtio-net-pci,id=nic-lagm1,netdev=net-lagm1,mac=52:54:00:00:0a:02,speed=10000,duplex=full",
  ]
}
