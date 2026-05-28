# This creates a Zedcloud datastore object that will point to a container
# registry (Dockerhub in this case as set by the `ds_fqdn` option) and with
# a specific path of "dockerhub username".
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

