module "default_project" {
  source = "../project"

  name        = var.project_name
  title       = "Default project (${var.project_name}"
  description = "A default project created in this enterprise"
}
