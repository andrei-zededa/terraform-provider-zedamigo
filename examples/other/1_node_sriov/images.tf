resource "zedcloud_image" "ubuntu_24_04_server_cloud_amd64" {
  name  = "ubuntu_24_04_server_cloud_amd64_${var.config_suffix}"
  title = "ubuntu_24_04_server_cloud_amd64_${var.config_suffix}"

  datastore_id = zedcloud_datastore.UBUNTU_CLOUD_IMAGES.id

  # The resulting URL for downloading the image will calculated by the edge-node as:
  #     ${ds_fqdn}/${ds_path}/${image_rel_url}
  #
  # So in this case, considering the `Local_HTTP_Server_8080` datastore it will be:
  #     http://192.168.192.168:8080/images_datastore/ubuntu_24_04_with_modbus_disk_999MB.qcow2
  image_rel_url    = "releases/noble/release-20260225/ubuntu-24.04-server-cloudimg-amd64.img"
  image_format     = "QCOW2"
  image_arch       = "AMD64"
  image_sha256     = "7AA6D9F5E8A3A55C7445B138D31A73D1187871211B2B7DA9DA2E1A6CBF169B21"
  image_size_bytes = 629048832
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
  image_rel_url    = "releases/noble/release-20260225/ubuntu-24.04-server-cloudimg-arm64.img"
  image_format     = "QCOW2"
  image_arch       = "ARM64"
  image_sha256     = "99E1D482B958E6BFD0183A4C48CE6DC334E09A3E29A4560F6F5FF85593D09D1D"
  image_size_bytes = 624109568
  image_type       = "IMAGE_TYPE_APPLICATION"
}
