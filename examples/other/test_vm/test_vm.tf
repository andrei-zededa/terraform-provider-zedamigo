resource "zedamigo_edge_node" "TEST_VM_01" {
  name               = var.VM_NAME
  cpus               = 4
  mem                = "4G"
  serial_no          = "1000"
  serial_port_server = true
  disk_image_base    = "/home/ev-zed1/ZED/testing/images_datastore/noble-server-cloudimg-amd64.daily.20250313.img"
  # disk_image_base = "/home/ev-zed1/ZED/testing/images_datastore/debian-12-genericcloud-amd64.qcow2"
  disk_size_mb = "100000" # ~ 100GB
  drive_if     = "virtio" # Required because the Debian `genericcloud` image doesn't have any drivers for IDE/SATA disks.

  extra_qemu_args = [
    "-drive", "file=${zedamigo_cloud_init_iso.CI_DATA_01.filename},format=raw,if=virtio", # Same comment as for `drive_if` above.
  ]
}
