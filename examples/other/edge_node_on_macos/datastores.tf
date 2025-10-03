resource "zedcloud_datastore" "UBUNTU_CLOUD_IMAGES" {
  name  = "Ubuntu_Cloud_Images_${var.config_suffix}"
  title = "Ubuntu_Cloud_Images_${var.config_suffix}"

  ds_type = "DATASTORE_TYPE_HTTPS"

  # The datastore FQDN+PATH are used when an edge-node tries to download an image. 
  # The resulting URL for downloading the image will calculated by the edge-node as: 
  #     ${ds_fqdn}/${ds_path}/${image_rel_url}
  ds_fqdn = "https://cloud-images.ubuntu.com/"
  ds_path = ""
}
