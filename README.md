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
to create the Zedcloud objects). This basically automates the [custom EVE-OS eve_installers instructions](https://help.zededa.com/hc/en-us/articles/26755679942939-Get-a-custom-EVE-OS-image):
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

Run the EVE-OS installation process which uses the previously created custom
installer ISO and will create all the partitions on the *empty* disk image.
This simulates booting a system (server/PC/NUC/etc.) with the installer ISO
and waiting for the automated EVE-OS installation process to finish. The *device*
serial number needs to match the one configured in the Zedcloud edge-node object
so that the onboarding will work correctly:
```hcl
resource "zedamigo_installed_edge_node" "EN_TEST_01_INSTALLED_EVE" {
  name          = "EN_TEST_01_INSTALLED_EVE"
  serial_no     = zedcloud_edgenode.EN_TEST_01.serialno
  installer_iso = zedamigo_eve_installer.eve_os_installer_iso_1451.filename
  disk_image_base = zedamigo_disk_image.empty_disk_100G.filename
}
```

Start the VM that will run the installed EVE-OS instance, it will have by default
a single NIC with an embedded QEMU DHCP server and internet access therefore
EVE-OS will be able to connect to Zedcloud and the edge-node should be successfully
onboarded and `ONLINE` in Zedcloud:
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
```
# Install QEMU
❯ sudo apt update
sudo apt install qemu-system-x86-64

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
```
❯ sudo install -m 0755 -d /etc/apt/keyrings                                                                           \
    && curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o - | sudo tee /etc/apt/keyrings/docker.asc         \
    && sudo chmod a+r /etc/apt/keyrings/docker.asc                                                                  \
    && curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg \
    && sudo chmod a+r /etc/apt/keyrings/docker.gpg                                                                  \
    && echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu $(. /etc/os-release && echo ${UBUNTU_CODENAME:-$VERSION_CODENAME}) stable" | sudo tee /etc/apt/sources.list.d/docker.list  \
    && sudo apt-get update                                                                                          \
    && sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin     \
    && sudo systemctl enable docker                                                                                 \
    && sudo systemctl start docker;

# Check the groups of the current user and assign it the docker group.
❯ id
uid=1000(ubnt) gid=1000(ubnt) groups=1000(ubnt),4(adm),24(cdrom),27(sudo),30(dip),46(plugdev),101(lxd)
❯ sudo usermod -aG docker $(whoami)
```

### Install OpenTofu
```
curl -fsSL https://github.com/opentofu/opentofu/releases/download/v1.10.6/tofu_1.10.6_linux_amd64.tar.gz -o tofu_1.10.6_linux_amd64.tar.gz  \
    && mkdir tofu_1.10.6_linux_amd64/                                                                                                       \
    && tar -xzvf tofu_1.10.6_linux_amd64.tar.gz -C tofu_1.10.6_linux_amd64/                                                                 \
    && mkdir -p ~/bin/                                                                                                                      \
    && mv tofu_1.10.6_linux_amd64/tofu ~/bin/                                                                                               \
    && ln -s ~/bin/tofu ~/bin/tf                                                                                                            \
    && rm -rf ./tofu_1.10.6_linux_amd64*;

```

### Install the zedamigo terraform provider locally
```
mkdir -p ~/.terraform.d/plugins/localhost/andrei-zededa/zedamigo/;
curl -fsSL https://github.com/andrei-zededa/terraform-provider-zedamigo/releases/download/v0.5.0/terraform-provider-zedamigo_0.5.0_linux_amd64.zip  \
    -o ~/.terraform.d/plugins/localhost/andrei-zededa/zedamigo/terraform-provider-zedamigo_0.5.0_linux_amd64.zip;
```

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
