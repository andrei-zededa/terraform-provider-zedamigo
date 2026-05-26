resource "zedcloud_image" "ubuntu_24_04_server_cloud_amd64" {
  name  = "ubuntu_24_04_server_cloud_amd64_${var.config_suffix}"
  title = "ubuntu_24_04_server_cloud_amd64_${var.config_suffix}"

  datastore_id = zedcloud_datastore.UBUNTU_CLOUD_IMAGES.id

  # The resulting URL for downloading the image will calculated by the edge-node as:
  #     ${ds_fqdn}/${ds_path}/${image_rel_url}
  #
  # So in this case, considering the `Local_HTTP_Server_8080` datastore it will be:
  #     http://192.168.192.168:8080/images_datastore/ubuntu_24_04_with_modbus_disk_999MB.qcow2
  image_rel_url    = "noble/20260518/noble-server-cloudimg-amd64.img"
  image_format     = "QCOW2"
  image_arch       = "AMD64"
  image_sha256     = "53FDDE898FEED8B027D94BAA9CFE8229867F330A1D9C49DC7D84465EE7F229F7"
  image_size_bytes = 627923968
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
  image_rel_url    = "noble/20260518/noble-server-cloudimg-arm64.img"
  image_format     = "QCOW2"
  image_arch       = "ARM64"
  image_sha256     = "6A61B967BA4A27DD1966F835A67643073ED55C2860CE3DC1CB0517282E6B8BEC"
  image_size_bytes = 624220672
  image_type       = "IMAGE_TYPE_APPLICATION"
}
