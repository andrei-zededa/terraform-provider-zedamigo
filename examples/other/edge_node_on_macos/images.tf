resource "zedcloud_image" "ubuntu_24_04_server_cloud" {
  name  = "ubuntu_24_04_server_cloud_${var.config_suffix}"
  title = "ubuntu_24_04_server_cloud_${var.config_suffix}"

  datastore_id = zedcloud_datastore.UBUNTU_CLOUD_IMAGES.id

  # The resulting URL for downloading the image will calculated by the edge-node as: 
  #     ${ds_fqdn}/${ds_path}/${image_rel_url}
  #
  # So in this case, considering the `Local_HTTP_Server_8080` datastore it will be:
  #     http://192.168.192.168:8080/images_datastore/ubuntu_24_04_with_modbus_disk_999MB.qcow2
  image_rel_url    = "noble/20251001/noble-server-cloudimg-arm64.img"
  image_format     = "QCOW2"
  image_arch       = "ARM64"
  image_sha256     = "88B381E23C422D4C625D8FB24D3D5BD03339C642C77BCB75F317CBEF0DEDD50F"
  image_size_bytes = 614701568
  image_type       = "IMAGE_TYPE_APPLICATION"
}
