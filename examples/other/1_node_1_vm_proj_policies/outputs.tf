#### The network-instances and the edge-app-instance are created by Zedcloud
#### itself, not by Terraform, so there is no resource to read their ids from.
#### What we can do is print the names under which they are expected to show up
#### in Zedcloud, which is enough to find them in the UI or via `zcli`.

output "AUTO_DEPLOYED_NETWORK_INSTANCES" {
  description = "Names of the network-instances which the project network-policy creates on the edge-node"
  value = [
    "${local.NI_LOCAL_NAT_NAME}.${zedcloud_edgenode.ENODE_TEST_AAAA.name}",
    "${local.NI_SWITCH_ETH1_NAME}.${zedcloud_edgenode.ENODE_TEST_AAAA.name}",
    "${local.NI_SWITCH_ETH2_NAME}.${zedcloud_edgenode.ENODE_TEST_AAAA.name}",
  ]
}

output "AUTO_DEPLOYED_EDGE_APP_INSTANCE" {
  description = "Name of the edge-app-instance which the project app-policy deploys on the edge-node, plus the credentials baked into its cloud-init"
  sensitive   = true
  value = {
    # `APP_NAMING_SCHEME_APP_DEVICE` => `<name_app_part>.<edge-node name>`.
    name     = "ubuntu_test.${zedcloud_edgenode.ENODE_TEST_AAAA.name}"
    username = local.UBUNTU_CLOUD_INIT_OVERRIDES["USERNAME"].value
    password = random_password.vm_password.result
  }
}
