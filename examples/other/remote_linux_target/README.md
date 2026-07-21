# Remote Linux target (run VMs on a NUC from your workstation)

This example runs Terraform/OpenTofu (and the zedamigo provider) on your
workstation — including macOS — while the QEMU VMs execute on a **remote Linux
host** (e.g. an Intel NUC) over SSH. You keep your config, state and tooling on
your workstation; the NUC just provides native Linux + KVM with the full feature
set (networking resources, nested virtualization, etc.).

## How it works

- The provider detects the target host's OS over SSH (`uname`). A Linux target
  selects the executor-based **QEMU** backend, so every command, file operation
  and socket happens on the NUC. macOS/vfkit is only used for a local target.
- No `qemu`/`vfkit` needs to be installed on your workstation — the tools are
  looked up and run on the target.
- The self-invoked helper daemons (gvproxy, dhcp, socket-tailer) need a Linux
  provider binary on the target:
  - **Released versions** are auto-bootstrapped via the install script.
  - **Local dev builds** are cross-compiled to `linux/amd64` on your workstation
    and uploaded automatically — build/install with `make dev-install`.
  - Or set `ssh.remote_binary_path` to a binary you place on the target yourself.

## Usage

```shell
# Build + install the provider locally (from the repo root):
make dev-install   # source = "localhost/andrei-zededa/zedamigo", version = 0.0.1

# Edit main.tf: set `target`, the `ssh {}` block, and the variables. Then:
tofu init
tofu apply
```

Verify the VM really runs on the NUC:

```shell
ssh nuc.lan 'ps auxww | grep qemu-system-x86_64'
ssh nuc.lan 'ls ~/.local/state/zedamigo/edge_nodes/'
```
