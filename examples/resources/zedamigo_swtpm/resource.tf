resource "zedamigo_swtpm" "example" {
  name = "edge_node_01_tpm"

  # Optional, "running" (the default) or "stopped".
  # state = "running"
}

# The swtpm control socket is meant to be used as the `swtpm_socket`
# attribute of an edge node resource, for example:
#
# resource "zedamigo_installed_edge_node" "example" {
#   # ...
#   swtpm_socket = zedamigo_swtpm.example.socket
# }
