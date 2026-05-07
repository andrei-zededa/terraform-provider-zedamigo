resource "zedcloud_image" "ubuntu_24_04_server_cloud_amd64" {
  name  = "ubuntu_24_04_server_cloud_amd64_${var.config_suffix}"
  title = "ubuntu_24_04_server_cloud_amd64_${var.config_suffix}"

  datastore_id = zedcloud_datastore.UBUNTU_CLOUD_IMAGES.id

  # The resulting URL for downloading the image will calculated by the edge-node as:
  #     ${ds_fqdn}/${ds_path}/${image_rel_url}
  #
  # So in this case, considering the `Local_HTTP_Server_8080` datastore it will be:
  #     http://192.168.192.168:8080/images_datastore/ubuntu_24_04_with_modbus_disk_999MB.qcow2
  image_rel_url    = "noble/20260323/noble-server-cloudimg-amd64.img"
  image_format     = "QCOW2"
  image_arch       = "AMD64"
  image_sha256     = "6E7016F2C9F4D3C00F48789EB6B9043BA2172CCC1B6B1EAF3ED1E29DD3E52BB3"
  image_size_bytes = 629987328
  image_type       = "IMAGE_TYPE_APPLICATION"
}

resource "zedcloud_image" "ubuntu_24_04_server_cloud_arm64" {
  name  = "ubuntu_24_04_server_cloud_arm64_${var.config_suffix}"
  title = "ubuntu_24_04_server_cloud_arm64_${var.config_suffix}"

  datastore_id = zedcloud_datastore.UBUNTU_CLOUD_IMAGES.id

  # The resulting URL for downloading the image will calculated by the edge-node as: 
  #     ${ds_fqdn}/${ds_path}/${image_rel_url}
  #
  # So in this case, considering the `Local_HTTP_Server_8080` datastore it will be:
  #     http://192.168.192.168:8080/images_datastore/ubuntu_24_04_with_modbus_disk_999MB.qcow2
  image_rel_url    = "noble/20260323/noble-server-cloudimg-arm64.img"
  image_format     = "QCOW2"
  image_arch       = "ARM64"
  image_sha256     = "C7EFF9B3EE6E7B212882E680A9E06CAC939107FBF5298384340A0AD1C667A38A"
  image_size_bytes = 626050048
  image_type       = "IMAGE_TYPE_APPLICATION"
}
