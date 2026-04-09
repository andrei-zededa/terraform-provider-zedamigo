data "zedcloud_model" "enterprise" {
  name        = "DSP-521"
  brand_id    = ""
  title       = ""
  type        = ""
  state       = ""
  attr        = {}
  origin_type = ""
}

locals {
  eth_io_members = [
    for label in sort([for m in data.zedcloud_model.enterprise.io_member_list : m.phylabel if m.ztype == "IO_TYPE_ETH"]) :
    [for m in data.zedcloud_model.enterprise.io_member_list : m if m.phylabel == label][0]
  ]
}

output "io_member_list_original" {
  value = data.zedcloud_model.enterprise.io_member_list
}

output "io_member_list_sorted_filtered" {
  value = local.eth_io_members
}
