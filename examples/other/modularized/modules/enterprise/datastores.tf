module "datastore_ubuntu_cloud" {
  source = "../datastore"

  name    = "Ubuntu_Cloud_Images${local.us_name_suffix}"
  ds_type = "DATASTORE_TYPE_HTTPS"
  ds_fqdn = "https://cloud-images.ubuntu.com"
  ds_path = ""
}

module "datastore_dockerhub" {
  count  = var.dockerhub_username != "" ? 1 : 0
  source = "../datastore"

  name = "Dockerhub_with_username${local.us_name_suffix}"

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
  ds_type = "DATASTORE_TYPE_CONTAINERREGISTRY"
  ds_fqdn = "docker://docker.io"
  ds_path = var.dockerhub_username
}
