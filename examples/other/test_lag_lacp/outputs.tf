output "host_bond" {
  description = "Host-side LAG (Linux bond) interface name."
  value       = zedamigo_lag.BOND_0.name
}

output "vm_ssh_port" {
  description = "Localhost TCP port forwarded to the VM's SSH (22) via nic0. SSH with: ssh -p <port> <username>@127.0.0.1"
  value       = zedamigo_edge_node.TEST_VM_01.ssh_port
}

output "vm_serial_socket" {
  description = "UNIX socket for the VM serial console. Attach with: socat - UNIX-CONNECT:<path>"
  value       = zedamigo_edge_node.TEST_VM_01.serial_port_socket
}
