resource "zedcloud_datastore" "LOCAL_DATASTORE" {
  name  = "Local_HTTP_Server_${var.config_suffix}"
  title = "Local_HTTP_Server_${var.config_suffix}"

  ds_type = "DATASTORE_TYPE_HTTP"

  # The datastore FQDN+PATH are used when an edge-node tries to download an image. 
  # The resulting URL for downloading the image will calculated by the edge-node as: 
  #     ${ds_fqdn}/${ds_path}/${image_rel_url}
  ds_fqdn = "http://${zedamigo_local_datastore.LOCAL_DS.listen}"
  ds_path = "images"
}

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
