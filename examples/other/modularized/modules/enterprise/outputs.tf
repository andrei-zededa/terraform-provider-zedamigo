# Names (for vessel data source lookups)
output "project_name" {
  description = "Project name"
  value       = zedcloud_project.this.name
}

output "model_name" {
  description = "Model name"
  value       = zedcloud_model.qemu_vm.name
}

output "brand_name" {
  description = "Brand name"
  value       = zedcloud_brand.qemu.name
}

output "image_names" {
  description = "Map of image keys to names"
  value = {
    ubuntu_24_04 = zedcloud_image.ubuntu_24_04.name
  }
}

output "app_names" {
  description = "Map of app keys to names"
  value = {
    ubuntu_vm = zedcloud_application.ubuntu_vm.name
  }
}

output "datastore_names" {
  description = "Map of datastore keys to names"
  value = merge(
    { ubuntu_cloud = module.datastore_ubuntu_cloud.name },
    length(module.datastore_dockerhub) > 0 ? { docker_hub = module.datastore_dockerhub[0].name } : {}
  )
}

output "network_name" {
  description = "Default network name"
  value       = zedcloud_network.default_network_dhcp_client.name
}

# IDs (for convenience)
output "project_id" {
  description = "Project ID"
  value       = zedcloud_project.this.id
}

output "model_id" {
  description = "Model ID"
  value       = zedcloud_model.qemu_vm.id
}

output "brand_id" {
  description = "Brand ID"
  value       = zedcloud_brand.qemu.id
}

output "image_ids" {
  description = "Map of image keys to IDs"
  value = {
    ubuntu_24_04 = zedcloud_image.ubuntu_24_04.id
  }
}

output "app_ids" {
  description = "Map of app keys to IDs"
  value = {
    ubuntu_vm = zedcloud_application.ubuntu_vm.id
  }
}

output "datastore_ids" {
  description = "Map of datastore keys to IDs"
  value = merge(
    { ubuntu_cloud = module.datastore_ubuntu_cloud.id },
    length(module.datastore_dockerhub) > 0 ? { docker_hub = module.datastore_dockerhub[0].id } : {}
  )
}

output "network_id" {
  description = "Default network ID"
  value       = zedcloud_network.default_network_dhcp_client.id
}
