# A Zedcloud datastore of type `DATASTORE_TYPE_CONTAINERREGISTRY` points to a
# container registry (`ds_fqdn`) *and* to one specific namespace inside it
# (`ds_path`). Any image linked to such a datastore then only specifies the
# container image repository name and tag in its `image_rel_url`, because the
# edge-node calculates the final URL as:
#     ${ds_fqdn}/${ds_path}/${image_rel_url}
#
# That means one datastore per registry *namespace*, which is why there are
# four of them here.
#
# Why not Docker Hub: this lab's egress firewall refuses connections to
# `auth.docker.io` (Cloudflare-fronted), and without its token service every
# anonymous Docker Hub pull dies at the manifest step — the app-instances sit
# in "error getting manifest ... connect: connection refused" forever and the
# downloader never records a single blob, which silently defeats the crash
# reproduction this example exists for (see the README). The registries below
# were verified end-to-end (token, manifest, blob redirect) from this lab:
#
#   - docker.elastic.co  — Elastic's own registry: elasticsearch / kibana /
#     logstash, deep version history, no pull rate limits;
#   - public.ecr.aws     — AWS ECR Public's `docker/library` namespace, a
#     mirror of the Docker Hub official images (identical digests): redis and
#     postgres, including the old major versions.
resource "zedcloud_datastore" "ELASTIC_ELASTICSEARCH" {
  name  = "Elastic_elasticsearch_${var.config_suffix}"
  title = "Elastic_elasticsearch_${var.config_suffix}"

  ds_type = "DATASTORE_TYPE_CONTAINERREGISTRY"
  ds_fqdn = "docker://docker.elastic.co"
  ds_path = "elasticsearch"
}

resource "zedcloud_datastore" "ELASTIC_KIBANA" {
  name  = "Elastic_kibana_${var.config_suffix}"
  title = "Elastic_kibana_${var.config_suffix}"

  ds_type = "DATASTORE_TYPE_CONTAINERREGISTRY"
  ds_fqdn = "docker://docker.elastic.co"
  ds_path = "kibana"
}

resource "zedcloud_datastore" "ELASTIC_LOGSTASH" {
  name  = "Elastic_logstash_${var.config_suffix}"
  title = "Elastic_logstash_${var.config_suffix}"

  ds_type = "DATASTORE_TYPE_CONTAINERREGISTRY"
  ds_fqdn = "docker://docker.elastic.co"
  ds_path = "logstash"
}

resource "zedcloud_datastore" "ECR_PUBLIC_LIBRARY" {
  name  = "ECR_public_library_${var.config_suffix}"
  title = "ECR_public_library_${var.config_suffix}"

  ds_type = "DATASTORE_TYPE_CONTAINERREGISTRY"
  ds_fqdn = "docker://public.ecr.aws"
  ds_path = "docker/library"
}
