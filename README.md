# zedamigo - A terraform provider to manage EVE-OS edge-nodes running as QEMU VMs

This ONLY works on a bare-metal Linux system because it starts VMs which are
running EVE-OS and EVE-OS starts VMs which are running the app instances.

**1 level** of nested virtualization works fine.

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
