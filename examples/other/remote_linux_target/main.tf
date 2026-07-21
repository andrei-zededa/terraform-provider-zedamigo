terraform {
  required_providers {
    zedamigo = {
      source = "localhost/andrei-zededa/zedamigo"
    }
  }
}

# Run the provider on your workstation (e.g. macOS) but execute all the QEMU VMs
# on a remote Linux host (e.g. an Intel NUC) over SSH. Nothing (qemu/vfkit) needs
# to be installed locally — all tools are looked up and run on the target. The
# provider auto-detects that the target is Linux and uses the QEMU backend.
provider "zedamigo" {
  target = "nuc.lan" # hostname or IP of the remote Linux host

  ssh {
    user             = "andrei"
    private_key_file = "~/.ssh/id_ed25519"
    # known_hosts_file = "~/.ssh/known_hosts" # default; host key is verified

    # Tunnel through a bastion if the target is not directly reachable:
    # proxy_jump = "root@bastion:22"

    # For a locally-built dev provider (`make dev-install`), the provider
    # cross-compiles a Linux binary and uploads it automatically. For a released
    # version it is bootstrapped via the install script. Override if needed:
    # remote_binary_path = "/home/andrei/bin/terraform-provider-zedamigo"
  }
}

# From here on the configuration is identical to a local setup — the resources
# are created ON THE TARGET.

resource "zedamigo_disk_image" "empty" {
  name    = "empty_100G"
  size_mb = 100000
}

resource "zedamigo_eve_installer" "installer" {
  name            = "EVE-OS_14.5.1"
  tag             = "14.5.1-lts-kvm-amd64"
  cluster         = var.ZEDEDA_CLOUD_URL
  authorized_keys = var.edge_node_ssh_pub_key
  grub_cfg        = <<-EOF
   set_getty
   set_global dom0_extra_args "$dom0_extra_args console=hvc0 hv_console=hvc0 dom0_console=hvc0"
   EOF
}

resource "zedamigo_installed_edge_node" "node_inst" {
  name            = "node01"
  serial_no       = "0001"
  installer_iso   = zedamigo_eve_installer.installer.filename
  disk_image_base = zedamigo_disk_image.empty.filename
}

resource "zedamigo_edge_node" "node" {
  name               = "node01"
  cpus               = "2"
  mem                = "2G"
  serial_no          = zedamigo_installed_edge_node.node_inst.serial_no
  serial_port_server = true
  disk_image_base    = zedamigo_installed_edge_node.node_inst.disk_image
  ovmf_vars_src      = zedamigo_installed_edge_node.node_inst.ovmf_vars
}

variable "ZEDEDA_CLOUD_URL" {
  type = string
}

variable "edge_node_ssh_pub_key" {
  type = string
}
