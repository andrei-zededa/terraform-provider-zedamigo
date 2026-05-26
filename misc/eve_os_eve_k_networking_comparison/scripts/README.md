# EVE connectivity checkers

Two scripts that verify, end-to-end, that a VM app-instance running on an EVE
node is wired correctly: the node's uplinks, the per-NIC host plumbing, the
guest IPs, and what Zedcloud reports must all agree.

- `check_eve_classic.py` — for classic EVE-OS (KVM). Expects each app NIC
  on the host to be a TAP enslaved to a Linux bridge.
- `check_eve_k.py` — for EVE-K (KubeVirt). Additionally enters the
  virt-launcher pod's network namespace and walks the
  `tap<h> → k6t-<h> → <h>-nic → nbu*x1 → host bridge` chain.

Both rely on `_eve_check_common.py` (shared helpers).

## Requirements

- Python 3.8+ (standard library only — `urllib`, `subprocess`, `dataclasses`).
- `ssh` in `$PATH`.
- An SSH key (or agent identity) that is authorized on the EVE node and on the
  guest VM. EVE uses the key configured as `debug.enable.ssh`; the guest VM is
  typically provisioned with the same key via cloud-init.
- A Zedcloud API token with read access to the project.

## Configuration

Both scripts take CLI flags. Most flags also read from environment variables
so you can `source` an `.env` file. The variables the scripts look at:

```
HOST_ADDR              # optional jump host
HOST_USER              # jump user (default "ubnt")
HOST_AUTH_SSH_KEY      # private key for the jump host
NODE_SSH_HOST          # EVE node SSH host
NODE_SSH_PORT          # EVE node SSH port
NODE_SSH_USER          # default "root"
NODE_SSH_KEY           # default = HOST_AUTH_SSH_KEY, else agent
VM_SSH_HOST            # default = NODE_SSH_HOST
VM_SSH_PORT            # default = NODE_SSH_PORT + 1
VM_SSH_USER            # default "labuser"
VM_SSH_KEY             # default = HOST_AUTH_SSH_KEY, else agent
ZEDEDA_CLOUD           # e.g. zedcloud.<cluster>.zededa.net
ZEDEDA_TOKEN           # Zedcloud API token
```

Run either script with `-h` for the full flag list.

## Examples

### 1. Direct SSH to a node on your network

EVE node at `10.0.0.5`, app-instance VM at `10.0.0.6`:

```bash
export ZEDEDA_CLOUD=zedcloud.gmwtus.zededa.net
export ZEDEDA_TOKEN=...

python3 scripts/check_eve_classic.py \
    --node-host 10.0.0.5 --node-port 22 --node-user root \
    --vm-host   10.0.0.6 --vm-port   22 --vm-user labuser
```

### 2. Through a jump host (port-forwarded EVE on `localhost`)

This is the typical lab setup: a single Ubuntu server (`ubnt`) is running the
EVE node as a QEMU VM, with its SSH port forwarded to a port on the server
(here `36426`). The guest VM gets the next port (`36427`).

```bash
# host_running_eve_os_vms.env:
#   export HOST_ADDR=198.51.100.141
#   export HOST_USER=ubnt
#   export HOST_AUTH_SSH_KEY=.ssh/id-ed25519-for-ubnt
source host_running_eve_os_vms.env
source zedcloud_creds.env       # ZEDEDA_CLOUD / ZEDEDA_TOKEN

# classic EVE-OS
python3 scripts/check_eve_classic.py --node-port 36426

# EVE-K standalone
python3 scripts/check_eve_k.py --node-port 45773
```

### 3. Jump host for the node, direct SSH to the VM

Useful when the guest has its own routable management address.

```bash
source host_running_eve_os_vms.env
source zedcloud_creds.env

python3 scripts/check_eve_k.py \
    --node-port 45773 \
    --vm-direct --vm-host 192.168.50.4 --vm-port 22
```

`--node-direct` does the symmetric thing for the node.

### 4. Override auto-discovered UUIDs

The scripts call `eve uuid` on the node and look at `appInstances` in the
node's Zedcloud status to find the app UUID. If you want to pin them:

```bash
python3 scripts/check_eve_classic.py --node-port 36426 \
    --node-uuid 17e2bb0f-798f-49c4-9bdf-1e4c365e10a8 \
    --app-uuid  42529890-7664-47a5-a610-314dff9c7a82
```

## Exit code

`0` if no `FAIL` lines; `1` otherwise. `WARN`s do not fail the check.

## Output

Each section prints a small banner, then a stream of `PASS` / `WARN` / `FAIL`
/ `INFO` lines and a final summary, e.g.:

```
Per-interface cross-check
=========================

  app_eth0  mac=06:99:10:1b:a8:14  zc_ips=['10.3.0.128']  zc_gw=['10.3.0.1']
    chain: guest:enp1s0 → tap:tap6c270ef2f25 → k6t:k6t-6c270ef2f25
         → pod-nic:6c270ef2f25-nic → host:nbu1x1(veth) → bridge:bn3
PASS  app_eth0: host handle is a veth (EVE-K)
PASS  app_eth0: pod-side veth→k6t bridge→tap chain present (...)
PASS  app_eth0: guest IPs ['10.3.0.128'] include Zedcloud ['10.3.0.128']

Summary
=======
  PASS=29  WARN=0  FAIL=0
```

## Graceful degradation

If the VM is unreachable, the script keeps going: it still prints the node
view, the host-side plumbing (and for EVE-K the pod-side chain), the
KubeVirt VMI status, and the Zedcloud-reported NICs. The missing pieces are
flagged as `WARN`, not `FAIL`.

If Zedcloud is unreachable, the per-interface cross-check is skipped (it
cannot run without controller data) and a `FAIL` is recorded on the
controller-fetch step.
