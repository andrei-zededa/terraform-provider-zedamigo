resource "zedcloud_datastore" "UBUNTU_CLOUD_IMAGES" {
  name  = "Ubuntu_Cloud_Images_${var.config_suffix}"
  title = "Ubuntu_Cloud_Images_${var.config_suffix}"

  ds_type = "DATASTORE_TYPE_HTTPS"

  # The datastore FQDN+PATH are used when an edge-node tries to download an image.
  # The resulting URL for downloading the image will calculated by the edge-node as:
  #     ${ds_fqdn}/${ds_path}/${image_rel_url}
  ds_fqdn = "https://cloud-images.ubuntu.com"
  ds_path = ""
}

resource "zedcloud_datastore" "GITHUB_LFEDGE_EVE_RELEASES" {
  name  = "GITHUB_LFEDGE_EVE_RELEASES_${var.config_suffix}"
  title = "GITHUB_LFEDGE_EVE_RELEASES_${var.config_suffix}"

  ds_type = "DATASTORE_TYPE_HTTPS"

  # The datastore FQDN+PATH are used when an edge-node tries to download an image.
  # The resulting URL for downloading the image will calculated by the edge-node as:
  #     ${ds_fqdn}/${ds_path}/${image_rel_url}
  #
  # In the case of trying to download the rootfs.img directly from Github the download
  # URL will look like this: http://github.com/lf-edge/eve/releases/download/14.5.0-lts/amd64.kvm.generic.rootfs.img
  # thefore we specify here `http://github.com/lf-edge/eve/releases/download` and
  # an image must specify as it's relative URL `14.5.0-lts/amd64.kvm.generic.rootfs.img`.
  ds_fqdn = "https://github.com"
  ds_path = "lf-edge/eve/releases/download"
}
