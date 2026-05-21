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

resource "zedcloud_edgenode" "ENODE_001" {
  name  = "ENODE_001_${var.config_suffix}"
  title = "ENODE_001"
  # Usually we would prefer to set a unique serial number like this and then
  # use it for the corresponding zedamigo_installed_edge_node and zedcloud_edgenode
  # resources as QEMU will set if through SMBIOS and then it will be detected by
  # EVE-OS as a "hardware serial number" (dmidecode system-serial-number). However
  # on macOS due to limitations of the Apple Virtualization Framework we cannot
  # set it, therefore we need to flip the logic. We let the EVE-OS install run
  # and generate a "soft serial" and use it here.
  # serialno       = "SN_TEST_AAAA_${var.config_suffix}"
  serialno       = zedamigo_installed_edge_node.ENODE_001.soft_serial
  onboarding_key = var.onboarding_key
  model_id       = zedcloud_model.QEMU_VM.id
  project_id     = zedcloud_project.PROJECT.id
  admin_state    = "ADMIN_STATE_ACTIVE"

  config_item {
    key          = "debug.enable.ssh"
    string_value = var.edge_node_ssh_pub_key
    # Need to set this otherwise we keep getting diff with the info in Zedcloud.
    uint64_value = "0"
  }

  interfaces {
    intfname   = "eth0"
    intf_usage = "ADAPTER_USAGE_MANAGEMENT"
    net_dhcp   = "NETWORK_DHCP_TYPE_CLIENT"
    cost       = 0
    netname    = zedcloud_network.edge_node_as_dhcp_client.name
    ztype      = "IO_TYPE_ETH"
    tags       = {}
  }

  interfaces {
    intfname   = "eth1"
    intf_usage = "ADAPTER_USAGE_APP_SHARED"
    net_dhcp   = "NETWORK_DHCP_TYPE_CLIENT"
    cost       = 0
    netname    = zedcloud_network.edge_node_as_dhcp_client.name
    ztype      = "IO_TYPE_ETH"
    tags       = {}
  }

  interfaces {
    intfname   = "eth2"
    intf_usage = "ADAPTER_USAGE_APP_SHARED"
    net_dhcp   = "NETWORK_DHCP_TYPE_CLIENT"
    cost       = 0
    netname    = zedcloud_network.edge_node_as_dhcp_client.name
    ztype      = "IO_TYPE_ETH"
    tags       = {}
  }

  interfaces {
    intfname   = "eth3"
    intf_usage = "ADAPTER_USAGE_APP_SHARED"
    net_dhcp   = "NETWORK_DHCP_TYPE_CLIENT"
    cost       = 0
    netname    = zedcloud_network.edge_node_as_dhcp_client.name
    ztype      = "IO_TYPE_ETH_PF"
    tags       = {}
  }

  tags = {}
}

resource "zedcloud_edgenode" "ENODE_002" {
  name           = "ENODE_002_${var.config_suffix}"
  title          = "ENODE_002"
  serialno       = zedamigo_installed_edge_node.ENODE_002.soft_serial
  onboarding_key = var.onboarding_key
  model_id       = zedcloud_model.QEMU_VM.id
  project_id     = zedcloud_project.PROJECT.id
  admin_state    = "ADMIN_STATE_ACTIVE"

  config_item {
    key          = "debug.enable.ssh"
    string_value = var.edge_node_ssh_pub_key
    # Need to set this otherwise we keep getting diff with the info in Zedcloud.
    uint64_value = "0"
  }

  interfaces {
    intfname   = "eth0"
    intf_usage = "ADAPTER_USAGE_MANAGEMENT"
    net_dhcp   = "NETWORK_DHCP_TYPE_CLIENT"
    cost       = 0
    netname    = zedcloud_network.edge_node_as_dhcp_client.name
    ztype      = "IO_TYPE_ETH"
    tags       = {}
  }

  interfaces {
    intfname   = "eth1"
    intf_usage = "ADAPTER_USAGE_APP_SHARED"
    net_dhcp   = "NETWORK_DHCP_TYPE_CLIENT"
    cost       = 0
    netname    = zedcloud_network.edge_node_as_dhcp_client.name
    ztype      = "IO_TYPE_ETH"
    tags       = {}
  }

  interfaces {
    intfname   = "eth2"
    intf_usage = "ADAPTER_USAGE_APP_SHARED"
    net_dhcp   = "NETWORK_DHCP_TYPE_CLIENT"
    cost       = 0
    netname    = zedcloud_network.edge_node_as_dhcp_client.name
    ztype      = "IO_TYPE_ETH"
    tags       = {}
  }

  interfaces {
    intfname   = "eth3"
    intf_usage = "ADAPTER_USAGE_APP_SHARED"
    net_dhcp   = "NETWORK_DHCP_TYPE_CLIENT"
    cost       = 0
    netname    = zedcloud_network.edge_node_as_dhcp_client.name
    ztype      = "IO_TYPE_ETH_PF"
    tags       = {}
  }

  tags = {}
}

resource "zedcloud_edgenode" "ENODE_003" {
  name           = "ENODE_003_${var.config_suffix}"
  title          = "ENODE_003"
  serialno       = zedamigo_installed_edge_node.ENODE_003.soft_serial
  onboarding_key = var.onboarding_key
  model_id       = zedcloud_model.QEMU_VM.id
  project_id     = zedcloud_project.PROJECT.id
  admin_state    = "ADMIN_STATE_ACTIVE"

  config_item {
    key          = "debug.enable.ssh"
    string_value = var.edge_node_ssh_pub_key
    # Need to set this otherwise we keep getting diff with the info in Zedcloud.
    uint64_value = "0"
  }

  interfaces {
    intfname   = "eth0"
    intf_usage = "ADAPTER_USAGE_MANAGEMENT"
    net_dhcp   = "NETWORK_DHCP_TYPE_CLIENT"
    cost       = 0
    netname    = zedcloud_network.edge_node_as_dhcp_client.name
    ztype      = "IO_TYPE_ETH"
    tags       = {}
  }

  interfaces {
    intfname   = "eth1"
    intf_usage = "ADAPTER_USAGE_APP_SHARED"
    net_dhcp   = "NETWORK_DHCP_TYPE_CLIENT"
    cost       = 0
    netname    = zedcloud_network.edge_node_as_dhcp_client.name
    ztype      = "IO_TYPE_ETH"
    tags       = {}
  }

  interfaces {
    intfname   = "eth2"
    intf_usage = "ADAPTER_USAGE_APP_SHARED"
    net_dhcp   = "NETWORK_DHCP_TYPE_CLIENT"
    cost       = 0
    netname    = zedcloud_network.edge_node_as_dhcp_client.name
    ztype      = "IO_TYPE_ETH"
    tags       = {}
  }

  interfaces {
    intfname   = "eth3"
    intf_usage = "ADAPTER_USAGE_APP_SHARED"
    net_dhcp   = "NETWORK_DHCP_TYPE_CLIENT"
    cost       = 0
    netname    = zedcloud_network.edge_node_as_dhcp_client.name
    ztype      = "IO_TYPE_ETH_PF"
    tags       = {}
  }

  tags = {
    # For 2-node ENC configs.
    # tie-breaker = true
  }
}


#### This creates a QCOW2 disk image file which will be used for running the
#### QEMU VM with EVE-OS.
resource "zedamigo_disk_image" "empty_disk" {
  name    = "empty_disk"
  size_mb = 200000 # ~200GB
}

#### This creates a custom EVE-OS installer ISO, it basically runs
#### `docker run ... lfedge/eve installer_iso`.
resource "zedamigo_eve_installer" "eve_os_installer" {
  name            = "EVE-OS_kvm_${lower(var.EDGE_NODE_ARCH)}"
  tag             = "16.14.0-k-${lower(var.EDGE_NODE_ARCH)}"
  cluster         = var.ZEDEDA_CLOUD_URL
  authorized_keys = var.edge_node_ssh_pub_key
  grub_cfg        = <<-EOF
   set_getty
   # We need to set the console to the serial port. Originally we were using the
   # emulated ISA serial port in QEMU which is then available to the Linux guest
   # (EVE-OS) as ttyS0, however on macOS (with vfkit) only virtio-serial is available
   # which will be hvc0. QEMU is now also switched to virtio-serial.
   # set_global dom0_extra_args "$dom0_extra_args console=ttyS0 hv_console=ttyS0 dom0_console=ttyS0"
   set_global dom0_extra_args "$dom0_extra_args console=hvc0 hv_console=hvc0 dom0_console=hvc0"
   EOF
}

#### This will start a QEMU VM with the EVE-OS installer ISO previously
#### created and run the install process.
resource "zedamigo_installed_edge_node" "ENODE_001" {
  name = "ENODE_001_INSTALL_${var.config_suffix}"
  # See comment for zedcloud_edgenode.ENODE_TEST_AAAA.serialno .
  # serial_no       = zedcloud_edgenode.ENODE_TEST_AAAA.serialno
  serial_no       = "SN_${var.config_suffix}_001"
  installer_iso   = zedamigo_eve_installer.eve_os_installer.filename
  disk_image_base = zedamigo_disk_image.empty_disk.filename
}

resource "zedamigo_installed_edge_node" "ENODE_002" {
  name = "ENODE_001_INSTALL_${var.config_suffix}"
  # See comment for zedcloud_edgenode.ENODE_TEST_AAAA.serialno .
  # serial_no       = zedcloud_edgenode.ENODE_TEST_AAAA.serialno
  serial_no       = "SN_${var.config_suffix}_002"
  installer_iso   = zedamigo_eve_installer.eve_os_installer.filename
  disk_image_base = zedamigo_disk_image.empty_disk.filename
}

resource "zedamigo_installed_edge_node" "ENODE_003" {
  name = "ENODE_001_INSTALL_${var.config_suffix}"
  # See comment for zedcloud_edgenode.ENODE_TEST_AAAA.serialno .
  # serial_no       = zedcloud_edgenode.ENODE_TEST_AAAA.serialno
  serial_no       = "SN_${var.config_suffix}_003"
  installer_iso   = zedamigo_eve_installer.eve_os_installer.filename
  disk_image_base = zedamigo_disk_image.empty_disk.filename
}

#### This starts a QEMU VM with the disk onto which EVE-OS was installed basically
#### the zedamigo_installed_edge_node resource. The QEMU VM will be listening onto
#### a random port on `localhost` to allow for SSH access to EVE-OS. Find the port
#### with:
#
#      ❯ tofu state show zedamigo_edge_node.ENODE_TEST_VM
#      # zedamigo_edge_node.ENODE_TEST_VM:
#      resource "zedamigo_edge_node" "ENODE_TEST_VM" {
#          cpus               = 4
#          disk_image         = "/home/ev-zed1/.local/state/zedamigo/edge_nodes/f8086b9b-bfb5-4d11-8c70-77d4d0453e33/disk0.disk_img.qcow2"
#          disk_image_base    = "/home/ev-zed1/.local/state/zedamigo/installed_nodes/b99f1fae-3f51-4bda-933e-f9d29f01d857/disk0.disk_img.qcow2"
#          id                 = "f8086b9b-bfb5-4d11-8c70-77d4d0453e33"
#          mem                = "4G"
#          name               = "ENODE_TEST_VM_27791"
#          ovmf_vars          = "/home/ev-zed1/.local/state/zedamigo/edge_nodes/f8086b9b-bfb5-4d11-8c70-77d4d0453e33/UEFI_OVMF_VARS.bin"
#          ovmf_vars_src      = "/home/ev-zed1/.local/state/zedamigo/installed_nodes/b99f1fae-3f51-4bda-933e-f9d29f01d857/UEFI_OVMF_VARS.bin"
#          qmp_socket         = "unix:/home/ev-zed1/.local/state/zedamigo/edge_nodes/f8086b9b-bfb5-4d11-8c70-77d4d0453e33/qmp.socket,server,nowait"
#          serial_console_log = "/home/ev-zed1/.local/state/zedamigo/edge_nodes/f8086b9b-bfb5-4d11-8c70-77d4d0453e33/serial_console_run.log"
#          serial_no          = "SN_TEST_27791"
#          serial_port_server = true
#          serial_port_socket = "/home/ev-zed1/.local/state/zedamigo/edge_nodes/f8086b9b-bfb5-4d11-8c70-77d4d0453e33/serial_port.socket"
#          ssh_port           = 50277
#          vm_running         = true
#      }
#
#### `ssh_port` is the value. Also `serial_console_log` is all the output
#### produced by VM on it's serial console.
resource "zedamigo_edge_node" "ENODE_001" {
  name     = "ENODE_001_${var.config_suffix}"
  cpus     = 6
  cpu_pins = [14, 15, 4, 6, 10, 12]
  mem      = "16G"
  # See comment for zedcloud_edgenode.ENODE_TEST_AAAA.serialno .
  serial_no          = zedamigo_installed_edge_node.ENODE_001.serial_no
  serial_port_server = true
  disk_image_base    = zedamigo_installed_edge_node.ENODE_001.disk_image
  ovmf_vars_src      = zedamigo_installed_edge_node.ENODE_001.ovmf_vars

  extra_qemu_args = [
    # Plain virtio NIC mode.
    "-nic", "tap,id=vmnet1,ifname=${zedamigo_tap.TAP_A_1.name},script=no,downscript=no,model=virtio,mac=1E:94:C2:3F:A:01",
    "-nic", "tap,id=vmnet2,ifname=${zedamigo_tap.TAP_B_1.name},script=no,downscript=no,model=virtio,mac=1E:94:C2:3F:B:01",
    "-nic", "tap,id=vmnet3,ifname=${zedamigo_tap.TAP_C_1.name},script=no,downscript=no,model=virtio,mac=1E:94:C2:3F:C:01",
  ]
}

resource "zedamigo_edge_node" "ENODE_002" {
  name     = "ENODE_002_${var.config_suffix}"
  cpus     = 6
  cpu_pins = [2, 3, 4, 5, 6, 7]
  mem      = "16G"
  # See comment for zedcloud_edgenode.ENODE_TEST_AAAA.serialno .
  serial_no          = zedamigo_installed_edge_node.ENODE_002.serial_no
  serial_port_server = true
  disk_image_base    = zedamigo_installed_edge_node.ENODE_002.disk_image
  ovmf_vars_src      = zedamigo_installed_edge_node.ENODE_002.ovmf_vars

  extra_qemu_args = [
    # Plain virtio NIC mode.
    "-nic", "tap,id=vmnet1,ifname=${zedamigo_tap.TAP_A_2.name},script=no,downscript=no,model=virtio,mac=1E:84:C2:3F:A:02",
    "-nic", "tap,id=vmnet2,ifname=${zedamigo_tap.TAP_B_2.name},script=no,downscript=no,model=virtio,mac=1E:84:C2:3F:B:02",
    "-nic", "tap,id=vmnet3,ifname=${zedamigo_tap.TAP_C_2.name},script=no,downscript=no,model=virtio,mac=1E:84:C2:3F:C:02",
  ]
}

resource "zedamigo_edge_node" "ENODE_003" {
  name     = "ENODE_003_${var.config_suffix}"
  cpus     = 6
  cpu_pins = [8, 9, 10, 11, 12, 13]
  mem      = "16G"
  # See comment for zedcloud_edgenode.ENODE_TEST_AAAA.serialno .
  serial_no          = zedamigo_installed_edge_node.ENODE_003.serial_no
  serial_port_server = true
  disk_image_base    = zedamigo_installed_edge_node.ENODE_003.disk_image
  ovmf_vars_src      = zedamigo_installed_edge_node.ENODE_003.ovmf_vars

  extra_qemu_args = [
    # Plain virtio NIC mode.
    "-nic", "tap,id=vmnet1,ifname=${zedamigo_tap.TAP_A_3.name},script=no,downscript=no,model=virtio,mac=1E:97:C2:3F:A:03",
    "-nic", "tap,id=vmnet2,ifname=${zedamigo_tap.TAP_B_3.name},script=no,downscript=no,model=virtio,mac=1E:97:C2:3F:B:03",
    "-nic", "tap,id=vmnet3,ifname=${zedamigo_tap.TAP_C_3.name},script=no,downscript=no,model=virtio,mac=1E:97:C2:3F:C:03",
  ]
}

# Polls each edge node over SSH and waits for EVE-OS to report that all kube
# components are initialized (file /var/lib/all_components_initialized exists).
# Without this barrier the cluster formation below races EVE-OS bringing up
# its Kubernetes stack and fails or stalls.
resource "null_resource" "WAIT_KUBE_READY" {
  triggers = {
    enode_001_id = zedamigo_edge_node.ENODE_001.id
    enode_002_id = zedamigo_edge_node.ENODE_002.id
    enode_003_id = zedamigo_edge_node.ENODE_003.id
  }

  provisioner "local-exec" {
    interpreter = ["/bin/bash", "-c"]
    command     = <<-EOT
      set -u
      declare -A PORTS=(
        [ENODE_001]=${zedamigo_edge_node.ENODE_001.ssh_port}
        [ENODE_002]=${zedamigo_edge_node.ENODE_002.ssh_port}
        [ENODE_003]=${zedamigo_edge_node.ENODE_003.ssh_port}
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

resource "zedcloud_edgenode_cluster" "TEST_CLUSTER" {
  name       = "TEST_CLUSTER_${var.config_suffix}"
  title      = "TEST_CLUSTER_${var.config_suffix}"
  project_id = zedcloud_project.PROJECT.id

  depends_on = [null_resource.WAIT_KUBE_READY]

  nodes {
    id                = zedcloud_edgenode.ENODE_001.id
    cluster_interface = "eth1"
  }

  nodes {
    id                = zedcloud_edgenode.ENODE_002.id
    cluster_interface = "eth1"
  }

  nodes {
    id                = zedcloud_edgenode.ENODE_003.id
    cluster_interface = "eth1"
  }
}
