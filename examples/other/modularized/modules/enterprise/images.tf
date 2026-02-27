resource "zedcloud_image" "ubuntu_24_04" {
  name  = "ubuntu_24_04_server_cloud${local.us_name_suffix}"
  title = "ubuntu_24_04_server_cloud${local.us_name_suffix}"

  datastore_id = module.datastore_ubuntu_cloud.id

  # The resulting URL for downloading the image will be calculated by the edge-node as:
  #     ${ds_fqdn}/${ds_path}/${image_rel_url}
  image_rel_url    = "noble/20260225/noble-server-cloudimg-amd64.img"
  image_format     = "QCOW2"
  image_arch       = "AMD64"
  image_sha256     = "7aa6d9f5e8a3a55c7445b138d31a73d1187871211b2b7da9da2e1a6cbf169b21"
  image_size_bytes = 629048832
  image_type       = "IMAGE_TYPE_APPLICATION"
}
