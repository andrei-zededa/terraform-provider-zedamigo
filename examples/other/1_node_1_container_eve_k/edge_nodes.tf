variable "onboarding_key" {
  description = "Zedcloud onboarding key"
  type        = string
  default     = "5d0767ee-0547-4569-b530-387e526f8cb9"
}

resource "zedcloud_network" "edge_node_as_dhcp_client" {
  name  = "edge_node_as_dhcp_client_${var.config_suffix}"
  title = "edge_node_as_dhcp_client"
  kind  = "NETWORK_KIND_V4"

  project_id = zedcloud_project.PROJECT.id

  ip {
    dhcp = "NETWORK_DHCP_TYPE_CLIENT"
  }
  mtu = 1500
}

resource "zedcloud_edgenode" "ENODE_TEST" {
  name           = "ENODE_TEST_${var.config_suffix}"
  title          = "ENODE_TEST"
  serialno       = "SN_TEST_${var.config_suffix}"
  onboarding_key = var.onboarding_key
  model_id       = zedcloud_model.QEMU_VM.id
  project_id     = zedcloud_project.PROJECT.id
  admin_state    = "ADMIN_STATE_ACTIVE"

  config_item {
    key          = "debug.enable.ssh"
    string_value = var.ssh_pub_key
    # Need to set this otherwise we keep getting diff with the info in Zedcloud.
    uint64_value = "0"
  }

  interfaces {
    intfname   = "eth0"
    intf_usage = "ADAPTER_USAGE_MANAGEMENT"
    cost       = 0
    netname    = zedcloud_network.edge_node_as_dhcp_client.name
    net_dhcp   = "NETWORK_DHCP_TYPE_CLIENT"
    ztype      = "IO_TYPE_ETH"

    tags = {}
  }
  tags = {}
}

resource "zedamigo_eve_installer" "eve_os_installer_iso" {
  name            = "EVE-K_${lower(var.EDGE_NODE_ARCH)}"
  tag             = "main-pr-5989-k-${lower(var.EDGE_NODE_ARCH)}"
  cluster         = var.ZEDEDA_CLOUD_URL
  authorized_keys = var.ssh_pub_key
  grub_cfg        = <<-EOF
   set_getty
   set_global dom0_extra_args "$dom0_extra_args console=hvc0 hv_console=hvc0 dom0_console=hvc0"
   EOF
}

resource "zedamigo_disk_image" "empty_disk_100G" {
  name    = "empty_disk_100G"
  size_mb = 100000 # ~100GB
}

resource "zedamigo_installed_edge_node" "ENODE_TEST_INSTALL" {
  name            = "ENODE_TEST_INSTALL_${var.config_suffix}"
  serial_no       = zedcloud_edgenode.ENODE_TEST.serialno
  # installer_iso   = zedamigo_eve_installer.eve_os_installer_iso.filename
  installer_iso   = "${path.module}/installer.iso" 
  disk_image_base = zedamigo_disk_image.empty_disk_100G.filename
}

resource "zedamigo_edge_node" "ENODE_TEST_VM" {
  name               = "ENODE_TEST_VM_${var.config_suffix}"
  cpus               = 6
  mem                = "16G"
  serial_no          = zedamigo_installed_edge_node.ENODE_TEST_INSTALL.serial_no
  serial_port_server = true
  disk_image_base    = zedamigo_installed_edge_node.ENODE_TEST_INSTALL.disk_image
  ovmf_vars_src      = zedamigo_installed_edge_node.ENODE_TEST_INSTALL.ovmf_vars
}

# Polls each edge node over SSH and waits for EVE-OS to report that all kube
# components are initialized (file /var/lib/all_components_initialized exists).
# Without this barrier the cluster formation below races EVE-OS bringing up
# its Kubernetes stack and fails or stalls.
resource "null_resource" "WAIT_KUBE_READY" {
  triggers = {
    enode_001_id = zedamigo_edge_node.ENODE_TEST_VM.id
  }

  provisioner "local-exec" {
    interpreter = ["/bin/bash", "-c"]
    command     = <<-EOT
      set -u
      declare -A PORTS=(
        [ENODE_001]=${zedamigo_edge_node.ENODE_TEST_VM.ssh_port}
      )

      SSH_OPTS=(
        -o StrictHostKeyChecking=no
        -o UserKnownHostsFile=/dev/null
        -o ConnectTimeout=5
        -o LogLevel=ERROR
      )

      DEADLINE=$(( $(date +%s) + 1800 ))
      declare -A READY=()

      while :; do
        all_ready=1
        for n in "$${!PORTS[@]}"; do
          if [[ -n "$${READY[$n]:-}" ]]; then continue; fi
          port="$${PORTS[$n]}"
          if ssh "$${SSH_OPTS[@]}" -p "$port" root@localhost \
              'eve exec kube ls -l /var/lib/all_components_initialized' \
              >/dev/null 2>&1; then
            echo "[$(date -Is)] $n (port $port) ready."
            READY[$n]=1
          else
            all_ready=0
            echo "[$(date -Is)] $n (port $port) not ready yet."
          fi
        done
        if [[ "$all_ready" -eq 1 ]]; then
          echo "All edge nodes report kube readiness."
          exit 0
        fi
        if (( $(date +%s) >= DEADLINE )); then
          echo "Timed out after 30 minutes waiting for kube readiness." >&2
          exit 1
        fi
        sleep 15
      done
    EOT
  }
}
