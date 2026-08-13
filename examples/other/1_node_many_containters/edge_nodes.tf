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
  name  = "ENODE_TEST_${var.config_suffix}"
  title = "ENODE_TEST with many container app-instances"
  # Usually we would prefer to set a unique serial number like this and then
  # use it for the corresponding zedamigo_installed_edge_node and zedcloud_edgenode
  # resources as QEMU will set if through SMBIOS and then it will be detected by
  # EVE-OS as a "hardware serial number" (dmidecode system-serial-number). However
  # on macOS due to limitations of the Apple Virtualization Framework we cannot
  # set it, therefore we need to flip the logic. We let the EVE-OS install run
  # and generate a "soft serial" and use it here.
  # serialno       = "SN_TEST_${var.config_suffix}"
  serialno       = zedamigo_installed_edge_node.ENODE_TEST_INSTALL.soft_serial
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

  # eth0: the QEMU user-mode ("SLIRP") NIC. It is the only port of this
  # edge-node which actually reaches the controller and the internet, so it is
  # the cheapest one and it is what `uplink` — the port of the local (NAT-ed)
  # network-instance shared by all 27 edge-app-instances — resolves to for both
  # their outbound traffic and their inbound port maps.
  interfaces {
    intfname   = "eth0"
    intf_usage = "ADAPTER_USAGE_MANAGEMENT"
    net_dhcp   = "NETWORK_DHCP_TYPE_CLIENT"
    cost       = 0
    netname    = zedcloud_network.edge_node_as_dhcp_client.name
    ztype      = "IO_TYPE_ETH"
    tags       = {}
  }

  # eth1..eth9: nine more management ports, each one a DHCP client of the DHCP
  # server running in its own host network namespace (host_networking.tf). They
  # share the one `NETWORK_KIND_V4` / DHCP-client network object above — it
  # carries no addressing, so there is nothing per-port in it to differ.
  dynamic "interfaces" {
    for_each = local.EXTRA_MGMT_PORTS

    content {
      intfname   = interfaces.key
      intf_usage = "ADAPTER_USAGE_MANAGEMENT"
      net_dhcp   = "NETWORK_DHCP_TYPE_CLIENT"
      cost       = interfaces.value.cost
      netname    = zedcloud_network.edge_node_as_dhcp_client.name
      ztype      = "IO_TYPE_ETH"
      tags       = {}
    }
  }

  tags = {}
}

#### This creates a QCOW2 disk image file which will be used for running the
#### QEMU VM with EVE-OS. It needs to be large enough for the EVE-OS partitions,
#### the container images of all the workloads and all of the volumes which the
#### 27 edge-app-instances ask for (43GB, see `workloads.tf`).
resource "zedamigo_disk_image" "empty_disk" {
  name    = "empty_disk_${var.config_suffix}"
  size_mb = var.EDGE_NODE_DISK_MB
}

#### This creates a custom EVE-OS installer ISO, it basically runs
#### `docker run ... lfedge/eve installer_iso`.
resource "zedamigo_eve_installer" "eve_os_installer" {
  name            = "EVE-OS_kvm_${lower(var.EDGE_NODE_ARCH)}"
  tag             = "16.0.1-lts-kvm-${lower(var.EDGE_NODE_ARCH)}"
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
resource "zedamigo_installed_edge_node" "ENODE_TEST_INSTALL" {
  name = "ENODE_TEST_INSTALL_${var.config_suffix}"
  # See comment for zedcloud_edgenode.ENODE_TEST.serialno .
  # serial_no       = zedcloud_edgenode.ENODE_TEST.serialno
  serial_no       = "1234567890"
  installer_iso   = zedamigo_eve_installer.eve_os_installer.filename
  disk_image_base = zedamigo_disk_image.empty_disk.filename
}

#### This starts a QEMU VM with the disk onto which EVE-OS was installed basically
#### the zedamigo_installed_edge_node resource. The QEMU VM will be listening onto
#### a random port on `localhost` to allow for SSH access to EVE-OS. Find the port
#### with `tofu state show zedamigo_edge_node.ENODE_TEST_VM` and look at
#### `ssh_port` / `nic0_port_forwards`. Also `serial_console_log` is all the
#### output produced by the VM on it's serial console.
####
#### Note that this is a *big* VM: with the defaults of `vars.tf` it asks for
#### 80 vCPUs and 256GB of RAM, so the host has to be sized accordingly. See the
#### comment in `terraform.tf` about running this against a remote host.
resource "zedamigo_edge_node" "ENODE_TEST_VM" {
  name = "ENODE_TEST_VM_${var.config_suffix}"
  cpus = var.EDGE_NODE_CPUS
  mem  = "${var.EDGE_NODE_MEM_GB}G"
  # See comment for zedcloud_edgenode.ENODE_TEST.serialno .
  serial_no          = zedamigo_installed_edge_node.ENODE_TEST_INSTALL.serial_no
  serial_port_server = true
  disk_image_base    = zedamigo_installed_edge_node.ENODE_TEST_INSTALL.disk_image
  ovmf_vars_src      = zedamigo_installed_edge_node.ENODE_TEST_INSTALL.ovmf_vars

  # eth1..eth9, each one backed by the TAP of its own host network namespace.
  #
  # The obvious spelling — one `-nic tap,...` per port — does not fit here.
  # `-nic` creates a *board* NIC, and a QEMU machine has only `MAX_NICS` == 8
  # of those slots; nic0 takes one, so the eighth TAP dies with:
  #
  #   -nic tap,id=vmnet8,...: no more on-board/default NIC slots available
  #
  # So the extra ports are `-netdev` + `-device` pairs instead, plugged into a
  # `pci-bridge` (32 slots) behind a `pcie-root-port` on q35's `pcie.0`. The
  # same topology as [`udp_traffic`](../udp_traffic/), which needs eight TAPs
  # of its own on top of nic0. In the guest:
  #
  #   -[0000:00]-+-00.0  82G33/G31/P35/P31 Express DRAM Controller
  #              +-02.0  Red Hat, Inc. Virtio network device          <- eth0
  #              +-10.0-[01-02]----00.0-[02]--+-01.0  Virtio network  <- eth1
  #              |                            +-02.0  Virtio network  <- eth2
  #              |                            \- ...                  <- ...
  #              \-1f.x  ICH9 LPC / SATA / SMBus
  #
  # Ordering still lands ethN on the port with index N, for two reasons: board
  # NICs are realized at machine init, i.e. before any `-device`, so nic0 stays
  # eth0 no matter where these appear on the command line; and the guest
  # enumerates the rest by PCI address, so pinning slot == port index (`addr`
  # 0x1..0x9) is what fixes the naming — not the argv order. `flatten` is
  # because each NIC is two argv entries.
  extra_qemu_args = concat(
    [
      "-device", "pcie-root-port,id=pcie1,bus=pcie.0,addr=0x10",
      "-device", "pci-bridge,id=pci1,bus=pcie1,chassis_nr=1",
    ],
    flatten([
      for name, port in local.EXTRA_MGMT_PORTS : [
        "-device",
        "virtio-net-pci,netdev=vmnet${port.index},mac=8c:84:74:10:99:${format("%02x", port.index)},bus=pci1,addr=${format("0x%x", port.index)}",
      ]
    ]),
    flatten([
      for name, port in local.EXTRA_MGMT_PORTS : [
        "-netdev",
        "tap,id=vmnet${port.index},ifname=${zedamigo_tap.MGMT[name].name},script=no,downscript=no",
      ]
    ]),
  )
}

locals {
  # `qmp_socket` is the value of the QEMU `-qmp` argument, e.g.
  # `unix:/var/lib/zedamigo/edge_nodes/<id>/qmp.socket,server,nowait`. The
  # script below wants only the socket path out of it.
  QMP_SOCKET_PATH = trimsuffix(trimprefix(zedamigo_edge_node.ENODE_TEST_VM.qmp_socket, "unix:"), ",server,nowait")
}

#### Barrier: wait until Zedcloud reports that the edge-node has actually come
#### up — `runState` BOOTING or ONLINE, i.e. no longer merely PROVISIONED or
#### REGISTERED — and then take the link of the QEMU user-mode NIC (eth0) down
#### over QMP, leaving the node with only the nine dead-end ports of
#### `local.EXTRA_MGMT_PORTS`.
####
#### The script runs ON THE PROVIDER TARGET, which is what makes this work when
#### `target` is a remote host: the QMP socket only exists next to the QEMU
#### process. It needs `curl`, `jq` and `socat` there.
####
#### NOTE: `var.ZEDEDA_CLOUD_TOKEN` is interpolated into the script, so it ends
#### up both in the Terraform state and in the script file which the provider
#### writes under `<lib_path>/wait_until/<id>/` on the target. That is the only
#### way to get a credential into a `zedamigo_wait_until` probe today — use a
#### token you are willing to have at rest in those two places.
resource "zedamigo_wait_until" "DISABLE_SLIRP_NIC" {
  triggers = {
    # Referencing both ids orders this after the VM (whose QMP socket it talks
    # to) and after the Zedcloud edge-node (whose state it polls), and re-runs
    # the barrier if either is recreated.
    node_vm_id = zedamigo_edge_node.ENODE_TEST_VM.id
    node_id    = zedcloud_edgenode.ENODE_TEST.id
  }

  # Generous: this covers the whole of the EVE-OS boot and onboarding, which on
  # a node this size is not quick.
  timeout  = "45m"
  interval = "20s"
  # Backstop only — both the curl and the socat below are self-bounding.
  attempt_timeout = "60s"

  script = <<-EOT
    set -u

    # `zedamigo_wait_until` injects no PATH and a non-interactive remote shell
    # may hand this script a minimal one, so *add to* whatever PATH we were
    # given. Do not replace it: on a target which does not populate the usual
    # directories at all — NixOS, where the tools live in
    # /run/current-system/sw/bin — replacing PATH is exactly how you lose curl
    # on a host where `command -v curl` works fine in a login shell. The extra
    # directories are appended only if they exist, so this stays a no-op
    # everywhere else.
    PATH="$${PATH:+$PATH:}/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
    for d in /run/wrappers/bin /run/current-system/sw/bin \
             /nix/var/nix/profiles/default/bin \
             "$${HOME:-/nonexistent}/.nix-profile/bin" /opt/homebrew/bin; do
      if [ -d "$d" ]; then PATH="$PATH:$d"; fi
    done
    export PATH

    CLUSTER='${var.ZEDEDA_CLOUD_URL}'
    TOKEN='${var.ZEDEDA_CLOUD_TOKEN}'
    NODE_ID='${zedcloud_edgenode.ENODE_TEST.id}'
    NODE_NAME='${zedcloud_edgenode.ENODE_TEST.name}'
    QMP_SOCKET='${local.QMP_SOCKET_PATH}'

    # A missing tool is not recoverable by retrying, but the resource has no
    # notion of a fatal exit, so it will be reported once per attempt until the
    # overall timeout expires.
    for tool in curl jq socat; do
      if ! command -v "$tool" >/dev/null 2>&1; then
        echo "required tool not found on the provider target: $tool (PATH=$PATH)" >&2
        exit 1
      fi
    done

    #### 1. Has the node reported in to Zedcloud?
    ####
    #### `runState` is what the Zedcloud UI shows in the edge-node Status
    #### column. An edge-node which has been created and onboarded but has not
    #### reported anything yet sits in PROVISIONED (or REGISTERED, before it is
    #### activated); BOOTING is the first state that means EVE-OS is running
    #### and talking to the controller.
    ####
    #### `/v1/devices/status-config` is a list endpoint, so filter it by name
    #### and then pick the record out by id — the id is the unambiguous key.
    STATE="$(curl -fsS --max-time 20 -G \
      -H "Authorization: Bearer $TOKEN" \
      -H 'Accept: application/json' \
      --data-urlencode "filter.deviceName=$NODE_NAME" \
      "https://$CLUSTER/api/v1/devices/status-config" \
      | jq -r --arg id "$NODE_ID" '.list[]? | select(.id == $id) | .runState')"

    if [ -z "$STATE" ]; then
      echo "no status-config record for $NODE_NAME ($NODE_ID) yet" >&2
      exit 1
    fi

    case "$STATE" in
      RUN_STATE_BOOTING | RUN_STATE_ONLINE)
        echo "edge-node is $STATE"
        ;;
      *)
        echo "edge-node is still $STATE" >&2
        exit 1
        ;;
    esac

    # Wait 2 mins (and a bit) so that the node has time to getit's new config.
    sleep 150;

    #### 2. Disable the QEMU user-mode NIC.
    ####
    #### `usernet0` is the id the provider gives the default nic0 netdev, and
    #### `set_link` on a netdev propagates to the NIC it is peered with, so
    #### this is a carrier loss on eth0 as far as EVE-OS is concerned. The NIC
    #### is not unplugged: the interface stays, the port numbering of the guest
    #### does not shift, and the same command with `"up":true` puts it back.
    ####
    #### QMP accepts nothing before the capabilities handshake, so both
    #### commands go down the same connection. `-t 5` keeps socat reading for
    #### five seconds after it has written them; re-running it is harmless.
    QMP_OUT="$(printf '%s\n%s\n' \
      '{"execute":"qmp_capabilities"}' \
      '{"execute":"set_link","arguments":{"name":"usernet0","up":false}}' \
      | socat -t 5 - "UNIX-CONNECT:$QMP_SOCKET")"

    # The greeting, then one `{"return": {}}` per command.
    if [ "$(printf '%s\n' "$QMP_OUT" | grep -c '"return"')" -lt 2 ]; then
      echo "unexpected QMP reply from $QMP_SOCKET: $QMP_OUT" >&2
      exit 1
    fi

    echo "set_link usernet0 down: $QMP_OUT"
  EOT
}
