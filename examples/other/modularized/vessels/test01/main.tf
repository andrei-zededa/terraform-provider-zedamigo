module "vessel" {
  source                  = "../../modules/vessel"
  enterprise_project_name = var.enterprise_project_name
  network_name            = var.network_name
  vessel_project_name     = var.vessel_project_name
  nodes                   = var.nodes
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

output "volume_instances" {
  value = module.vessel.volume_instances
}
