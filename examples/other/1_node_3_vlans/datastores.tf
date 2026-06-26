# Datastore for the Ubuntu cloud images used by the VM edge-app definition.
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

# This creates a Zedcloud datastore object that will point to a container
# registry (Dockerhub in this case as set by the `ds_fqdn` option) and with
# a specific path of "dockerhub username". Used by the container edge-app
# definition.
#
# Any image that is linked to this datastore needs to specify in it's
# `image_rel_url` option (image relative URL) only the container image
# repository name and possibly also the container image tag.
#
# For example if this datastore is created with:
#     ds_fqdn = "docker://docker.io"
#     ds_path = "andreizededa"
#
# Then an image can be created with:
#     image_rel_url = "hello-zedcloud:v1.2.3"
#
# Which is basically the equivalent of:
#     docker pull andreizededa/hello-zedcloud:v1.2.3
resource "zedcloud_datastore" "DOCKERHUB_WITH_USERNAME" {
  name  = "Dockerhub_${var.DOCKERHUB_USERNAME}_${var.config_suffix}"
  title = "Dockerhub_${var.DOCKERHUB_USERNAME}_${var.config_suffix}"

  ds_type = "DATASTORE_TYPE_CONTAINERREGISTRY"
  ds_fqdn = "docker://docker.io"
  ds_path = var.DOCKERHUB_USERNAME
}
