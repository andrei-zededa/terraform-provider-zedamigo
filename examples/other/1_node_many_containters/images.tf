# The container image tags which the workloads of `workloads.tf` use. They are
# kept in one place because the same value is needed both for the
# `image_rel_url` of the image object and for the `ac_version` /
# `user_defined_version` of every edge-app definition which references it.
#
# These are plain upstream images, pinned to a tag which existed at the time
# this example was written - bump them as needed.
locals {
  CONTAINER_IMAGE_TAGS = {
    elasticsearch = "8.17.4"
    kafka         = "3.9.1"
    kibana        = "8.17.4"
    postgres      = "17-alpine"
    redis         = "7.4-alpine"
  }
}

# `image_size_bytes` is left at 0 for all of these: for a container image
# Zedcloud cannot know the size upfront (it is a manifest of layers, not a single
# file) and it does not need to. A side effect is that the immutable
# ("content tree") volumes which Zedcloud creates implicitly for the image drive
# of every edge-app-instance are reported as 0 bytes, so they don't show up in
# the storage numbers of the edge-node - only the persistent and the dedicated
# volumes of `workloads.tf` do.
resource "zedcloud_image" "ELASTICSEARCH" {
  name  = "elasticsearch_container_image_${var.config_suffix}"
  title = "elasticsearch_container_image_${var.config_suffix}"

  datastore_id = zedcloud_datastore.DOCKERHUB_LIBRARY.id

  image_rel_url    = "elasticsearch:${local.CONTAINER_IMAGE_TAGS.elasticsearch}"
  image_format     = "CONTAINER"
  image_arch       = upper(var.EDGE_NODE_ARCH)
  image_type       = "IMAGE_TYPE_APPLICATION"
  image_size_bytes = 0
}

resource "zedcloud_image" "KIBANA" {
  name  = "kibana_container_image_${var.config_suffix}"
  title = "kibana_container_image_${var.config_suffix}"

  datastore_id = zedcloud_datastore.DOCKERHUB_LIBRARY.id

  image_rel_url    = "kibana:${local.CONTAINER_IMAGE_TAGS.kibana}"
  image_format     = "CONTAINER"
  image_arch       = upper(var.EDGE_NODE_ARCH)
  image_type       = "IMAGE_TYPE_APPLICATION"
  image_size_bytes = 0
}

resource "zedcloud_image" "REDIS" {
  name  = "redis_container_image_${var.config_suffix}"
  title = "redis_container_image_${var.config_suffix}"

  datastore_id = zedcloud_datastore.DOCKERHUB_LIBRARY.id

  image_rel_url    = "redis:${local.CONTAINER_IMAGE_TAGS.redis}"
  image_format     = "CONTAINER"
  image_arch       = upper(var.EDGE_NODE_ARCH)
  image_type       = "IMAGE_TYPE_APPLICATION"
  image_size_bytes = 0
}

resource "zedcloud_image" "POSTGRES" {
  name  = "postgres_container_image_${var.config_suffix}"
  title = "postgres_container_image_${var.config_suffix}"

  datastore_id = zedcloud_datastore.DOCKERHUB_LIBRARY.id

  image_rel_url    = "postgres:${local.CONTAINER_IMAGE_TAGS.postgres}"
  image_format     = "CONTAINER"
  image_arch       = upper(var.EDGE_NODE_ARCH)
  image_type       = "IMAGE_TYPE_APPLICATION"
  image_size_bytes = 0
}

# Kafka comes from the `apache` namespace, hence the other datastore. The
# resulting pull is the equivalent of `docker pull apache/kafka:3.9.1`.
resource "zedcloud_image" "KAFKA" {
  name  = "kafka_container_image_${var.config_suffix}"
  title = "kafka_container_image_${var.config_suffix}"

  datastore_id = zedcloud_datastore.DOCKERHUB_APACHE.id

  image_rel_url    = "kafka:${local.CONTAINER_IMAGE_TAGS.kafka}"
  image_format     = "CONTAINER"
  image_arch       = upper(var.EDGE_NODE_ARCH)
  image_type       = "IMAGE_TYPE_APPLICATION"
  image_size_bytes = 0
}

# Lookup table so that the `image` key of a workload role in `workloads.tf`
# ("kafka", "redis", ...) can be resolved to the corresponding image object.
locals {
  CONTAINER_IMAGES = {
    elasticsearch = zedcloud_image.ELASTICSEARCH
    kafka         = zedcloud_image.KAFKA
    kibana        = zedcloud_image.KIBANA
    postgres      = zedcloud_image.POSTGRES
    redis         = zedcloud_image.REDIS
  }
}
