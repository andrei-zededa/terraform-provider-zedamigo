# One Zedcloud image object per distinct `repo:tag` pair which the workload
# catalog of `workloads.tf` references — 27 of them, deliberately.
#
# This is the load-bearing part of reproducing the EVE-OS downloader crash (see
# the README): the downloader keeps one metrics entry per unique image blob URL
# it downloads within a boot session, publishes the whole map as a single
# pubsub message capped at 65535 bytes, and treats an oversize publish
# as *fatal* — which the watchdog then turns into a node reboot. Five images
# shared by 27 replicas dedup to only ~60 unique blobs, far below the cap. The
# 27 distinct tags of `workloads.tf` resolve to ~313 unique blobs / ~87KB of
# serialized metrics (verified against the docker.elastic.co and
# public.ecr.aws manifests, 2026-08), which is comfortably past the cap.
#
# Different *versions* of the same five repos are used rather than 27 different
# products: releases published far apart share almost no layers (only 43 of the
# 356 blob references dedup away), and it also matches the scenario — many
# versions/builds of the same product family downloading on one node in one session.
locals {
  # Which datastore each image repo of `workloads.tf` pulls from — one
  # datastore per registry namespace, see `datastores.tf` (including why none
  # of them is Docker Hub).
  CONTAINER_IMAGE_DATASTORES = {
    elasticsearch = zedcloud_datastore.ELASTIC_ELASTICSEARCH.id
    kibana        = zedcloud_datastore.ELASTIC_KIBANA.id
    logstash      = zedcloud_datastore.ELASTIC_LOGSTASH.id
    postgres      = zedcloud_datastore.ECR_PUBLIC_LIBRARY.id
    redis         = zedcloud_datastore.ECR_PUBLIC_LIBRARY.id
  }

  # The distinct `repo:tag` pairs of the workload catalog, e.g.
  # "elasticsearch:8.17.4". A `check` block in `workloads.tf` asserts that
  # there are as many of these as there are app-instances.
  CONTAINER_IMAGE_SET = toset([
    for w in local.WORKLOADS : "${w.image}:${w.image_tag}"
  ])
}

# `image_size_bytes` is left at 0 for all of these: for a container image
# Zedcloud cannot know the size upfront (it is a manifest of layers, not a
# single file) and it does not need to. A side effect is that the immutable
# ("content tree") volumes which Zedcloud creates implicitly for the image
# drive of every edge-app-instance are reported as 0 bytes, so they don't show
# up in the storage numbers of the edge-node - only the persistent and the
# dedicated volumes of `workloads.tf` do.
resource "zedcloud_image" "CONTAINER" {
  for_each = local.CONTAINER_IMAGE_SET

  # "elasticsearch:8.17.4" -> "elasticsearch_8-17-4_container_image_<suffix>".
  # The tag goes into the object name so that the 27 of them stay unique, with
  # the dots swapped out to keep the name unambiguous.
  name  = "${replace(replace(each.value, ":", "_"), ".", "-")}_container_image_${var.config_suffix}"
  title = "${replace(replace(each.value, ":", "_"), ".", "-")}_container_image_${var.config_suffix}"

  datastore_id = local.CONTAINER_IMAGE_DATASTORES[split(":", each.value)[0]]

  # The `repo:tag` pair *is* the relative URL under the datastore's namespace,
  # e.g. "elasticsearch:8.17.4" under the `elasticsearch` namespace of
  # docker.elastic.co is `docker pull docker.elastic.co/elasticsearch/elasticsearch:8.17.4`.
  image_rel_url    = each.value
  image_format     = "CONTAINER"
  image_arch       = upper(var.EDGE_NODE_ARCH)
  image_type       = "IMAGE_TYPE_APPLICATION"
  image_size_bytes = 0
}

# Lookup table so that a workload replica of `workloads.tf` can be resolved to
# its image object as `local.CONTAINER_IMAGES["<image>:<image_tag>"]`.
locals {
  CONTAINER_IMAGES = zedcloud_image.CONTAINER
}
