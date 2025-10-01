resource "zedcloud_image" "ubuntu_24_04_server_cloud" {
  name  = "ubuntu_24_04_server_cloud_${var.config_suffix}"
  title = "ubuntu_24_04_server_cloud_${var.config_suffix}"

  datastore_id = zedcloud_datastore.UBUNTU_CLOUD_IMAGES.id

  # The resulting URL for downloading the image will calculated by the edge-node as: 
  #     ${ds_fqdn}/${ds_path}/${image_rel_url}
  #
  # So in this case, considering the `Local_HTTP_Server_8080` datastore it will be:
  #     http://192.168.192.168:8080/images_datastore/ubuntu_24_04_with_modbus_disk_999MB.qcow2
  image_rel_url    = "noble/20250805/noble-server-cloudimg-amd64.img"
  image_format     = "QCOW2"
  image_arch       = "AMD64"
  image_sha256     = "834AF9CD766D1FD86ECA156DB7DFF34C3713FBBC7F5507A3269BE2A72D2D1820"
  image_size_bytes = 618925568
  image_type       = "IMAGE_TYPE_APPLICATION"
}
