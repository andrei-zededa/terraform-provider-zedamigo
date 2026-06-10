# LAG (Linux bond) with LACP — host `zedamigo_lag` peered with an Ubuntu VM

This example demonstrates the `zedamigo_lag` resource by building an 802.3ad
(LACP, fast rate) link-aggregation group on the host and peering it, link for
link, with a bond configured *inside* an Ubuntu VM via cloud-init.

## Topology

```
              host                                      Ubuntu VM (QEMU)
  ┌───────────────────────────┐                ┌───────────────────────────┐
  │ zedamigo_lag "lab-bond0"  │                │   netplan bond  "bond0"    │
  │  mode 802.3ad / lacp fast │                │  mode 802.3ad / lacp fast  │
  │  10.10.10.1/24            │                │  10.10.10.2/24             │
  │      ├── lab-lagm0 (TAP) ==════ QEMU tap ════== lagm0 (52:54:00:00:0a:01)│
  │      └── lab-lagm1 (TAP) ==════ QEMU tap ════== lagm1 (52:54:00:00:0a:02)│
  └───────────────────────────┘                │                            │
                                                │  nic0 (user-mode) → SSH    │
                                                └───────────────────────────┘
```

Each TAP is a point-to-point link between one host bond member and one VM bond
member. Because both ends speak 802.3ad, LACP negotiates over each link and the
two links aggregate into a single logical bond on each side.

The host members attach from the **TAP** side (`zedamigo_tap.master =
zedamigo_lag.BOND_0.name`), so the LAG leaves `enslaved_interfaces` unset — see
the note in `networking.tf`.

## Prerequisites

- The `zedamigo` provider installed locally as `localhost/andrei-zededa/zedamigo`
  (see the repo `install.sh`).
- KVM/QEMU available, and passwordless `sudo` for `ip` (the provider runs with
  `use_sudo = true`).
- An Ubuntu cloud image. Set `var.ubuntu_image` (see `vars.tf`) to its path, and
  set `var.user_ssh_pub_key` to your SSH public key.

## Apply

```sh
tofu init      # or: terraform init
tofu apply -var "user_ssh_pub_key=$(cat ~/.ssh/id_ed25519.pub)"
```

LACP only comes up once the VM has booted and QEMU has opened the TAPs (which
gives the host-side members carrier).

## Verify — host side

```sh
sudo ip -d link show lab-bond0          # mode 802.3ad, lacp_rate fast
cat /proc/net/bonding/lab-bond0         # Bonding Mode: IEEE 802.3ad; LACP rate: fast
                                        # two slaves, same Aggregator ID
ping -c3 10.10.10.2                      # reach the VM over the bond
```

## Verify — VM side

Get the forwarded SSH port (and serial socket) from the outputs:

```sh
tofu output                              # vm_ssh_port, vm_serial_socket, host_bond
ssh -p "$(tofu output -raw vm_ssh_port)" lab@127.0.0.1
# or attach to the serial console:
socat - "UNIX-CONNECT:$(tofu output -raw vm_serial_socket)"
```

Then, inside the VM:

```sh
cat /proc/net/bonding/bond0             # Bonding Mode: IEEE 802.3ad; LACP rate: fast
networkctl status bond0                 # CARRIER, both members enslaved
ip addr show bond0                      # 10.10.10.2/24
ping -c3 10.10.10.1                      # reach the host LAG over the bond
```

A healthy LACP bond reports, on both ends, the same aggregator with two ports in
`Churn State: none` / `Actor Churn` settled, and `MII Status: up` for each slave.

## Notes

- The VM NICs are matched in `cloud-init/network-config` by MAC address, so the
  guest's interface naming (`enp0sN`) does not matter. The management NIC is
  matched by QEMU's default user-mode MAC `52:54:00:12:34:56`.
- To experiment with the in-place member feature of `zedamigo_lag`, you can add
  a third host TAP/VM NIC pair and grow both bonds; the host side updates in
  place (no bond re-creation). Changing `mode`/`lacp_rate` re-creates the host
  bond.
