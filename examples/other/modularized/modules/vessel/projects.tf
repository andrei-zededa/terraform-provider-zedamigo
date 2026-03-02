module "vessel_project" {
  source = "../project"

  name        = var.vessel_project_name
  title       = "Vessel project (${var.vessel_project_name}"
  description = "A project created for a specific vessel"
}
