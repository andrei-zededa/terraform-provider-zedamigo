module "vessel" {
  source       = "../../modules/vessel"
  name_suffix  = var.name_suffix
  project_name = var.project_name
  model_name   = var.model_name
  network_name = var.network_name
  app_name     = var.app_name
  nodes        = var.nodes
}

output "edge_nodes" {
  value = module.vessel.edge_nodes
}

output "network_instances" {
  value = module.vessel.network_instances
}

output "app_instances" {
  value = module.vessel.app_instances
}
