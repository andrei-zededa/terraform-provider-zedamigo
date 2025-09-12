resource "zedcloud_image" "CONTAINER_IMAGE" {
  name  = "${var.DOCKERHUB_IMAGE_NAME}_container_image_${var.config_suffix}"
  title = "${var.DOCKERHUB_IMAGE_NAME}_container_image_${var.config_suffix}"

  datastore_id = zedcloud_datastore.DOCKERHUB_WITH_USERNAME.id
  # datastore_id_list = [
  #  zedcloud_datastore.DOCKERHUB_WITH_USERNAME.id
  # ]

  # The final URL that an edge-node will use to download this image will be
  #     ${ds_fqdn}/${ds_path}/${image_rel_url}
  #
  # See also the comment for the datastore.
  image_rel_url    = "${var.DOCKERHUB_IMAGE_NAME}:${var.DOCKERHUB_IMAGE_LATEST_TAG}"
  image_format     = "CONTAINER"
  image_arch       = "AMD64"
  image_type       = "IMAGE_TYPE_APPLICATION"
  image_size_bytes = 0
}

resource "zedcloud_image" "ubuntu_24_04_test" {
  name  = "ubuntu_24_04_test_${var.config_suffix}"
  title = "ubuntu_24_04_test_${var.config_suffix}"

  # This creates an implicit dependency on the `zedcloud_datastore.Local_HTTP_Server_8080`
  # resource, so a block like:
  #     depends_on = [
  #       zedcloud_datastore.Local_HTTP_Server_8080
  #     ]
  # is not needed.
  datastore_id = zedcloud_datastore.Local_HTTP_Server_9999.id
  # NOTE: Currently both `datastore_id` and `datastore_id_list` are needed even
  # though it's the same information.
  # datastore_id_list = [
  #   zedcloud_datastore.Local_HTTP_Server_9999.id
  # ]

  # The resulting URL for downloading the image will calculated by the edge-node as: 
  #     ${ds_fqdn}/${ds_path}/${image_rel_url}
  #
  # So in this case, considering the `Local_HTTP_Server_8080` datastore it will be:
  #     http://192.168.192.168:8080/images_datastore/ubuntu_24_04_with_modbus_disk_999MB.qcow2
  image_rel_url    = "ubuntu_24_04_with_modbus_disk_999MB.qcow2"
  image_format     = "QCOW2"
  image_arch       = "AMD64"
  image_sha256     = "CA1CD93D8863B08BA089559A11AE1F4C2B518B69B572FC629479F9052A747E19"
  image_size_bytes = 1018757120
  image_type       = "IMAGE_TYPE_APPLICATION"
}

resource "zedcloud_image" "ubuntu_24_04_server_cloud" {
  name  = "ubuntu_24_04_server_cloud_${var.config_suffix}"
  title = "ubuntu_24_04_server_cloud_${var.config_suffix}"

  datastore_id = zedcloud_datastore.UBUNTU_CLOUD_IMAGES.id

  # The resulting URL for downloading the image will calculated by the edge-node as: 
  #     ${ds_fqdn}/${ds_path}/${image_rel_url}
  #
  # So in this case, considering the `Local_HTTP_Server_8080` datastore it will be:
  #     http://192.168.192.168:8080/images_datastore/ubuntu_24_04_with_modbus_disk_999MB.qcow2
  image_rel_url    = "noble/20250805/noble-server-cloudimg-amd64.img"
  image_format     = "QCOW2"
  image_arch       = "AMD64"
  image_sha256     = "834AF9CD766D1FD86ECA156DB7DFF34C3713FBBC7F5507A3269BE2A72D2D1820"
  image_size_bytes = 618925568
  image_type       = "IMAGE_TYPE_APPLICATION"
}

resource "zedcloud_image" "ROOTFS_14_5_0_LTS" {
  name  = "14.5.0-lts-kvm-amd64"
  title = "14.5.0-lts-kvm-amd64"

  datastore_id = zedcloud_datastore.GITHUB_LFEDGE_EVE_RELEASES.id

  image_arch       = "AMD64"
  image_format     = "RAW"
  image_rel_url    = "14.5.0-lts/amd64.kvm.generic.rootfs.img"
  image_sha256     = "67d11d31c3c5e4e4b8b9d17e9354acc5f44752db4016443d45f52780deace63b" # Incorrect, MUST fix.
  image_size_bytes = "275316736"                                                        # Incorrect, MUST fix.
  image_type       = "IMAGE_TYPE_EVE"
}

resource "zedcloud_image" "ROOTFS_14_5_1_LTS" {
  name  = "14.5.1-lts-kvm-amd64"
  title = "14.5.1-lts-kvm-amd64"

  datastore_id = zedcloud_datastore.GITHUB_LFEDGE_EVE_RELEASES.id

  image_arch       = "AMD64"
  image_format     = "RAW"
  image_rel_url    = "14.5.1-lts/amd64.kvm.generic.rootfs.img"
  image_sha256     = "f9b7cdb1fd3bdd5b30306cf873698fcb29c9c5509c5c9f0941f9d8d20d63943f" # Incorrect, MUST fix.
  image_size_bytes = "275972096"                                                        # Incorrect, MUST fix.
  image_type       = "IMAGE_TYPE_EVE"
}
