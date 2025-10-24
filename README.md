# zedamigo - A terraform provider to manage EVE-OS edge-nodes running as QEMU VMs

*zedamigo* is a terraform provider which manages QEMU VMs on the local system.

It is useful primarily when those VMs run [EVE-OS](https://github.com/lf-edge/eve)
and are represented as [edge-nodes](https://help.zededa.com/hc/en-us/articles/4440282818715-Edge-Node-Overview)
in a [ZEDEDA Cloud instance](https://zededa.com/products/how-it-works/), also
known as a Zedcloud cluster or server.

*zedamigo* ONLY works on a bare-metal Linux system because most usecases involve
nested virtualization. Starting VMs which are running EVE-OS and EVE-OS starts VMs
which are running the [edge-app-instances](https://help.zededa.com/hc/en-us/articles/4440266233243-Manage-an-Edge-Application-Instance#h_01HV7H05VXB8WN1TF78Y72EEWG).

*zedamigo* can be used as a simple QEMU VM *manager* for any VM quests. But it
is most useful when those VM guests are running EVE-OS since then it can be
used together with the [zedcloud terraform provider](https://help.zededa.com/hc/en-us/articles/4440359495835-ZEDEDA-Terraform-Provider#h_01HCDRC294XF1HEZ8976SQW829),
[zedcloud provider docs](https://registry.terraform.io/providers/zededa/zedcloud/latest/docs)
and the setup can be managed end-to-end from a single terraform configuration
(both the VMs representing the edge-nodes but also all the Zedcloud objects).

## Common workflow example

Create an edge-node object in Zedcloud using the `zedcloud_edgenode` resource
(of course all the other Zedcloud objects like brand, model, project, etc. can
be managed also from the same terraform config but in this example we skip that):
```hcl
resource "zedcloud_edgenode" "EN_TEST_01" {
  name           = "EN_TEST_01"
  title          = "EN_TEST_01 created via terraform"
  serialno       = "SN_TEST_2615921012"
  onboarding_key = "5d0767ee-0547-4569-b530-387e526f8cb9"
  model_id       = "da032fcd-6774-4710-915c-56622dca65b1" 
  project_id     = "e918b0f1-31da-46c6-9486-fe900119a9ca" 
  admin_state    = "ADMIN_STATE_ACTIVE"

  config_item {
    key          = "debug.enable.ssh"
    string_value = var.edge_node_ssh_pub_key
    uint64_value = "0"
  }

  interfaces {
    intfname   = "eth0"
    intf_usage = "ADAPTER_USAGE_MANAGEMENT"
    cost       = 0
    netname    = "net_obj_node_dhcp_client" 
    tags = {}
  }

  tags = {}
}
```

Create an empty disk image using the `zedamigo_disk_image` resource:
```hcl
resource "zedamigo_disk_image" "empty_disk_100G" {
  name    = "empty_disk_100G"
  size_mb = 100000 # ~100GB
}
```

Create a custom EVE-OS installer ISO using the `zedamigo_eve_installer` resource,
specific for the Zedcloud cluster that we are using (the same on which the
[zedcloud terraform provider is configured](https://registry.terraform.io/providers/zededa/zedcloud/latest/docs#zedcloud_url-1)
to create the Zedcloud objects). This basically automates the [custom EVE-OS eve_installers instructions](https://help.zededa.com/hc/en-us/articles/26755679942939-Get-a-custom-EVE-OS-image).

> NOTE: Here we use the `var.ZEDEDA_CLOUD_URL` variable, the same one which
> should be used to configure the zedcloud terraform provider. This MUST be 
> set to the `zedcloud.*.zededa.net` variant and NOT to the `zedcontrol.*.zededa.net`
> variant.

Choosing which EVE-OS version will be used is done through the `tag` attribute
of the `zedamigo_eve_installer` resource. This is the container image tag of
the [lfedge/eve](https://hub.docker.com/r/lfedge/eve/tags) image from Dockerhub.
```hcl
resource "zedamigo_eve_installer" "eve_os_installer_iso_1451" {
  name            = "EVE-OS_14.5.1-lts-kvm-amd64"
  tag             = "14.5.1-lts-kvm-amd64"
  cluster         = var.ZEDEDA_CLOUD_URL
  authorized_keys = var.edge_node_ssh_pub_key
  grub_cfg        = <<-EOF
   set_getty
   set_global dom0_extra_args "$dom0_extra_args console=ttyS0 hv_console=ttyS0 dom0_console=ttyS0"
   EOF
}
```

Then using the `zedamigo_installed_edge_node` resource we run the EVE-OS
installation process which uses the previously created custom installer ISO
and which will create all the partitions on the *empty* disk image. This
simulates booting a system (server/PC/NUC/etc.) with the installer ISO and
waiting for the automated EVE-OS installation process to finish. The *device*
serial number needs to match the one configured in the Zedcloud edge-node object
so that the onboarding will work correctly.

> Notice how the different resources are linked together by using their attributes
> as variables in other resources. In terraform this creates an implicity dependency
> between 2 resources and terraform knows that it needs to create one resource
> before the other.

```hcl
resource "zedamigo_installed_edge_node" "EN_TEST_01_INSTALLED_EVE" {
  name          = "EN_TEST_01_INSTALLED_EVE"
  serial_no     = zedcloud_edgenode.EN_TEST_01.serialno
  installer_iso = zedamigo_eve_installer.eve_os_installer_iso_1451.filename
  disk_image_base = zedamigo_disk_image.empty_disk_100G.filename
}
```

Using the `zedamigo_edge_node` resource start the VM that will run the installed
EVE-OS instance, it will have by default a single NIC with an embedded QEMU DHCP
server and internet access therefore EVE-OS will be able to connect to Zedcloud
and the edge-node should be successfully onboarded and `ONLINE` in Zedcloud:
```hcl
resource "zedamigo_edge_node" "ENODE_TEST_VM_AAAA" {
  name               = "EN_TEST_01_VM"
  cpus               = "2"
  mem                = "2G"
  serial_no          = zedamigo_installed_edge_node.EN_TEST_01_INSTALLED_EVE.serial_no
  serial_port_server = true
  disk_image_base    = zedamigo_installed_edge_node.EN_TEST_01_INSTALLED_EVE.disk_image
  ovmf_vars_src      = zedamigo_installed_edge_node.EN_TEST_01_INSTALLED_EVE.ovmf_vars
}
```

## Setup on Ubuntu 24.04

### Install QEMU

```shell
# Install QEMU
sudo apt -y update && sudo apt install -y --no-install-recommends qemu-system-x86-64
```

```shell
# Verify what is the group of /dev/kvm, if it doesn't have a specific group
# this needs to be configured via udev rules.
❯ ls -lsah /dev/kvm
0 crw-rw---- 1 root kvm 10, 232 Sep 12 08:19 /dev/kvm

# Check the groups of the current user and assign it with the same kvm group.
❯ id
uid=1000(ubnt) gid=1000(ubnt) groups=1000(ubnt),4(adm),24(cdrom),27(sudo),30(dip),46(plugdev),101(lxd)
❯ sudo usermod -aG kvm $(whoami)

# Logout/login is needed for the user to retrieve the new group membership.
```

### Install Docker
```shell
sudo install -m 0755 -d /etc/apt/keyrings                                                                           \
    && curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o - | sudo tee /etc/apt/keyrings/docker.asc         \
    && sudo chmod a+r /etc/apt/keyrings/docker.asc                                                                  \
    && curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg \
    && sudo chmod a+r /etc/apt/keyrings/docker.gpg                                                                  \
    && echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu $(. /etc/os-release && echo ${UBUNTU_CODENAME:-$VERSION_CODENAME}) stable" | sudo tee /etc/apt/sources.list.d/docker.list  \
    && sudo apt-get update                                                                                          \
    && sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin     \
    && sudo systemctl enable docker                                                                                 \
    && sudo systemctl start docker;
```

```shell
# Check the groups of the current user and assign it the docker group.
❯ id
uid=1000(ubnt) gid=1000(ubnt) groups=1000(ubnt),4(adm),24(cdrom),27(sudo),30(dip),46(plugdev),101(lxd)
❯ sudo usermod -aG docker $(whoami)
```

### Install the zedamigo terraform provider locally

zedamigo works well both with *terraform* and *OpenTofu* (recent versions).
If either is already installed then the install script will use the already
installed version. If neither is found then the install script will install
the latest version of OpenTofu (and symlink it as `tf` for convenience).

> NOTE: There are several methods of using a terraform provider which is not
> published to a provider registry (https://registry.terraform.io/ or https://search.opentofu.org/).
> Here we rely on the fact that both `terraform` and `opentofu` will look for
> provider releases in the `~/.terraform.d` local directory first. However if
> you have any specific configuration (for example dev overrides in `~/.config/opentofu/tofurc`
or `~/.terraformrc`) then this might fail.

The install script should finish with a message like `OpenTofu has been successfully initialized!`.

```shell
curl -fsSL https://github.com/andrei-zededa/terraform-provider-zedamigo/releases/download/v0.5.4/install.sh | sh -s
```

```shell
Trying to install version 'latest' of the zedamigo terraform provider from https://github.com/andrei-zededa/terraform-provider-zedamigo (linux / amd64).
No terraform or opentofu found. Will try to install the latest opentofu release from https://github.com/opentofu/opentofu .

Initializing the backend...

Initializing provider plugins...
- Finding localhost/andrei-zededa/zedamigo versions matching "0.5.4"...
- Installing localhost/andrei-zededa/zedamigo v0.5.4...
- Installed localhost/andrei-zededa/zedamigo v0.5.4 (unauthenticated)

OpenTofu has created a lock file .terraform.lock.hcl to record the provider
selections it made above. Include this file in your version control repository
so that OpenTofu can guarantee to make the same selections by default when
you run "tofu init" in the future.

╷
│ Warning: Incomplete lock file information for providers
│
│ Due to your customized provider installation methods, OpenTofu was forced to calculate lock file checksums locally for the following providers:
│   - localhost/andrei-zededa/zedamigo
│
│ The current .terraform.lock.hcl file only includes checksums for linux_amd64, so OpenTofu running on another platform will fail to install these providers.
│
│ To calculate additional checksums for another platform, run:
│   tofu providers lock -platform=linux_amd64
│ (where linux_amd64 is the platform to generate)
╵

OpenTofu has been successfully initialized!

You may now begin working with OpenTofu. Try running "tofu plan" to see
any changes that are required for your infrastructure. All OpenTofu commands
should now work.

If you ever set or change modules or backend configuration for OpenTofu,
rerun this command to reinitialize your working directory. If you forget, other
commands will detect it and remind you to do so if necessary.
```

## Documentation

Each resource implemented by the zedamigo provider has documentation auto-generated
from the source code in the [docs/](docs/) folder.

> NOTE: currently *macOS* support is just planned, nothing works.

| Resource | Linux (amd64) | macOS (arm64) | Notes |
|---       |:---:          |:---:          |---    |
| [disk_image](docs/resources/disk_image.md) | ✅ | ❌ | |
| [eve_installer](docs/resources/eve_installer.md) | ✅ | ❌ | |
| [installed_edge_node](docs/resources/installed_edge_node.md) | ✅ | ❌ | |
| [edge_node](docs/resources/edge_node.md) | ✅ | ❌ | |
| [local_datastore](docs/resources/local_datastore.md) | ✅ | ❌ | Really just a simple Go web server |
| [cloud_init_iso](docs/resources/cloud_init_iso.md) | ✅ | ❌ | If `genisoimage` is installed |
| [bridge](docs/resources/bridge.md) | ✅ | ❌ | Needs `use_sudo = true` |
| [tap](docs/resources/tap.md) | ✅ | ❌ | Needs `use_sudo = true` |
| [vlan](docs/resources/vlan.md) | ✅ | ❌ | Needs `use_sudo = true` |
| [dhcp_server](docs/resources/dhcp_server.md) | ✅ | ❌ | Needs `use_sudo = true` |
| [dhcp6_server](docs/resources/dhcp6_server.md) | ✅ | ❌ | Needs `use_sudo = true` |
| [radv](docs/resources/radv.md) | ✅ | ❌ | Needs `use_sudo = true` |
| [virtual_machine](docs/resources/virtual_machine.md) | ✅ | ❌ | It's just an alias for edge_node |
| [vm](docs/resources/vm.md) | ✅ | ❌ | It's just an alias for edge_node |
| [swtpm](docs/resources/swtpm.md) | ❌ | ❌ | WIP, currently not working |

## Troubleshooting

If creating a resource fails then we can look at the command outputs for that
resource. Every resource will have an `id` (UUID) and a corresponding directory
created based on it's type. By default these will be created in `$HOME/.local/state/zedamigo/`:

```
❯ ls -lsah ~/.local/state/zedamigo/
total 85K
8.5K drwx------ 10 ev-zed1 users 10 Aug 27 16:57 .
8.5K drwxr-xr-x  7 ev-zed1 users  9 Sep 12 21:37 ..
8.5K drwx------ 24 ev-zed1 users 24 Sep 15 13:31 bridges
8.5K drwx------  8 ev-zed1 users  8 Sep 15 13:31 cloud_init_isos
8.5K drwx------  8 ev-zed1 users  8 Sep 12 12:15 disk_images
8.5K drwx------ 26 ev-zed1 users 26 Sep 15 13:31 edge_nodes
8.5K drwx------  2 ev-zed1 users  4 Aug 27 16:57 embedded_ovmf
8.5K drwx------ 14 ev-zed1 users 14 Sep 12 12:15 eve_installers
8.5K drwx------ 10 ev-zed1 users 10 Sep 12 12:15 installed_nodes
8.5K drwx------ 32 ev-zed1 users 32 Sep 15 13:31 tapIntfs
```

In this example creating the edge-node VM resource failed:

```
│ Warning: Edge Node Resource Read Error
│
│   with zedamigo_vm.CONTROL_PLANE_01,
│   on vms.tf line 25, in resource "zedamigo_vm" "CONTROL_PLANE_01":
│   25: resource "zedamigo_vm" "CONTROL_PLANE_01" {
│
│ Can't read EVE-OS console log: dial unix /home/ev-zed1/.local/state/zedamigo/edge_nodes/cdeca52a-04e8-4745-a6b1-8c05180cb3ec/qmp.socket: connect: no such file or directory
│
│ (and 3 more similar warnings elsewhere)
╵
╷
│ Error: Missing Resource State After Create
│
│   with zedamigo_vm.CONTROL_PLANE_01,
│   on vms.tf line 25, in resource "zedamigo_vm" "CONTROL_PLANE_01":
│   25: resource "zedamigo_vm" "CONTROL_PLANE_01" {
│
│ The Terraform Provider unexpectedly returned no resource state after having no errors in the resource creation. This is always an issue in the Terraform Provider and should be reported to the provider developers.
│
│ The resource may have been successfully created, but Terraform is not tracking it. Applying the configuration again with no other action may result in duplicate resource errors. Import the resource if the resource was actually created and Terraform should be tracking it.
```

We can get the resource `id` (UUID) and corresponding resource directory from
the warning output (like in the example above). Then we can look into that
directory for the command outputs. Since the error is about the QMP socket then
we known that this means that the corresponding QEMU command failed:

```
❯ cat /home/ev-zed1/.local/state/zedamigo/edge_nodes/cdeca52a-04e8-4745-a6b1-8c05180cb3ec/20250915_133110_qemu-system-x86_64_stderr.log
qemu-system-x86_64: -serial unix:/home/ev-zed1/.local/state/zedamigo/edge_nodes/cdeca52a-04e8-4745-a6b1-8c05180cb3ec/serial_port.socket,server,wait: info: QEMU waiting for connection on: disconnected:unix:/home/ev-zed1/.local/state/zedamigo/edge_nodes/cdeca52a-04e8-4745-a6b1-8c05180cb3ec/serial_port.socket,server=on
```

If we're unsure if the corresponding QEMU process is actually running or not
we can search for a process matching the resource `id` (UUID):

```
❯ ps auxwww | grep -i cdeca52a-04e8-4745-a6b1-8c05180cb3ec
ev-zed1  2879557  0.1  0.0 1690360 11980 pts/5   Sl   13:31   0:00 /home/ev-zed1/ZED/git/github.com/andrei-zededa/terraform-provider-zedamigo/terraform-provider-zedamigo -socket-tailer -st.connect /home/ev-zed1/.local/state/zedamigo/edge_nodes/cdeca52a-04e8-4745-a6b1-8c05180cb3ec/serial_port.socket -st.out /home/ev-zed1/.local/state/zedamigo/edge_nodes/cdeca52a-04e8-4745-a6b1-8c05180cb3ec/serial_console_run.log
ev-zed1  2879585  8.5  4.1 4715768 1330720 pts/5 Sl   13:31   1:10 /home/ev-zed1/.nix-profile/bin/qemu-system-x86_64 --name edge_node_master01 --enable-kvm -machine q35,accel=kvm,kernel-irqchip=split -nographic -m 2G -cpu host -smp 2 -device intel-iommu,intremap=on -smbios type=1,serial=1000,manufacturer=Dell Inc.,product=ProLiant 100 with 2 disks -drive if=pflash,format=raw,readonly=on,file=/home/ev-zed1/.local/state/zedamigo/embedded_ovmf/OVMF_CODE.fd -drive if=pflash,format=raw,file=/home/ev-zed1/.local/state/zedamigo/edge_nodes/cdeca52a-04e8-4745-a6b1-8c05180cb3ec/UEFI_OVMF_VARS.bin -qmp unix:/home/ev-zed1/.local/state/zedamigo/edge_nodes/cdeca52a-04e8-4745-a6b1-8c05180cb3ec/qmp.socket,server,nowait -pidfile /home/ev-zed1/.local/state/zedamigo/edge_nodes/cdeca52a-04e8-4745-a6b1-8c05180cb3ec/qemu.pid -nic user,id=usernet0,hostfwd=tcp::51778-:22,model=virtio -drive file=/home/ev-zed1/.local/state/zedamigo/edge_nodes/cdeca52a-04e8-4745-a6b1-8c05180cb3ec/disk0.disk_img.qcow2,format=qcow2,if=virtio -serial unix:/home/ev-zed1/.local/state/zedamigo/edge_nodes/cdeca52a-04e8-4745-a6b1-8c05180cb3ec/serial_port.socket,server,wait -drive file=/home/ev-zed1/.local/state/zedamigo/cloud_init_isos/047afac0-117e-42e4-8780-924b9a565636/CI_DATA_CP_01.iso,format=raw,if=virtio -nic tap,id=usernet1,ifname=k3s-master01,mac=06:00:00:00:10:00,script=no,downscript=no,model=virtio
ev-zed1  2882995  0.0  0.0 230708  2512 pts/5    S+   13:44   0:00 grep --color=auto -i cdeca52a-04e8-4745-a6b1-8c05180cb3ec
```
