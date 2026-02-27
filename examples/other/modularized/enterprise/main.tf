module "enterprise" {
  source      = "../modules/enterprise"
  name_suffix = var.name_suffix
}

output "project_name" {
  value = module.enterprise.project_name
}

output "model_name" {
  value = module.enterprise.model_name
}

output "brand_name" {
  value = module.enterprise.brand_name
}

output "image_names" {
  value = module.enterprise.image_names
}

output "app_names" {
  value = module.enterprise.app_names
}

output "datastore_names" {
  value = module.enterprise.datastore_names
}

output "network_name" {
  value = module.enterprise.network_name
}
