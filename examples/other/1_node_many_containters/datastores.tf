# A Zedcloud datastore of type `DATASTORE_TYPE_CONTAINERREGISTRY` points to a
# container registry (`ds_fqdn`) *and* to one specific namespace / "dockerhub
# username" inside it (`ds_path`). Any image linked to such a datastore then
# only specifies the container image repository name and tag in its
# `image_rel_url`, because the edge-node calculates the final URL as:
#     ${ds_fqdn}/${ds_path}/${image_rel_url}
#
# That means one datastore per registry *namespace*, which is why there are two
# of them here: `redis`, `postgres`, `elasticsearch` and `kibana` are Docker Hub
# official images and therefore live in the `library` namespace, while Kafka is
# published by the Apache project under the `apache` namespace.
resource "zedcloud_datastore" "DOCKERHUB_LIBRARY" {
  name  = "Dockerhub_library_${var.config_suffix}"
  title = "Dockerhub_library_${var.config_suffix}"

  ds_type = "DATASTORE_TYPE_CONTAINERREGISTRY"
  ds_fqdn = "docker://docker.io"
  ds_path = "library"
}

resource "zedcloud_datastore" "DOCKERHUB_APACHE" {
  name  = "Dockerhub_apache_${var.config_suffix}"
  title = "Dockerhub_apache_${var.config_suffix}"

  ds_type = "DATASTORE_TYPE_CONTAINERREGISTRY"
  ds_fqdn = "docker://docker.io"
  ds_path = "apache"
}
