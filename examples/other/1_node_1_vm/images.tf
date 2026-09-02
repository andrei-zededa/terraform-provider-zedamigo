resource "zedcloud_image" "ubuntu_24_04_server_cloud_amd64" {
  name  = "ubuntu_24_04_server_cloud_amd64_${var.config_suffix}"
  title = "ubuntu_24_04_server_cloud_amd64_${var.config_suffix}"

  datastore_id = zedcloud_datastore.UBUNTU_CLOUD_IMAGES.id

  # The resulting URL for downloading the image will calculated by the edge-node as:
  #     ${ds_fqdn}/${ds_path}/${image_rel_url}
  #
  # So in this case, considering the `Local_HTTP_Server_8080` datastore it will be:
  #     http://192.168.192.168:8080/images_datastore/ubuntu_24_04_with_modbus_disk_999MB.qcow2
  image_rel_url    = "noble/20260826/noble-server-cloudimg-amd64.img"
  image_format     = "QCOW2"
  image_arch       = "AMD64"
  image_sha256     = "D0FE84BB5F80853425FA6BE28E2C106F30104C3CFE8611933F2E65C9B63F0E30"
  image_size_bytes = 624829952
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
  image_rel_url    = "noble/20260826/noble-server-cloudimg-arm64.img"
  image_format     = "QCOW2"
  image_arch       = "ARM64"
  image_sha256     = "AFA139BAC6F2629C1E1F2F8F34215F3A9AD9779801BCB945521BA1A45016743F"
  image_size_bytes = 619036160
  image_type       = "IMAGE_TYPE_APPLICATION"
}
