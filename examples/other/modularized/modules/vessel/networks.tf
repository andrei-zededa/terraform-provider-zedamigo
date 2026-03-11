resource "zedcloud_network" "management" {
  count = var.management_network != null ? 1 : 0

  name       = var.management_network.name
  title      = coalesce(var.management_network.title, var.management_network.name)
  kind       = var.management_network.kind
  project_id = module.vessel_project.id

  enterprise_default = false

  ip {
    dhcp    = var.management_network.ip.dhcp
    subnet  = var.management_network.ip.subnet
    gateway = var.management_network.ip.gateway
    dns     = var.management_network.ip.dns
  }

  mtu = var.management_network.mtu
}
