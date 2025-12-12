locals {
  domainname    = "k3s-lab.example.net"
  control_plane = { name = "master01", serial = 1000, mac = "06:00:00:00:10:00", ipv4_addr = "172.27.213.141/25" }
  nodes = {
    node01 = { name = "node01", serial = 2001, mac = "06:00:00:00:20:01", ipv4_addr = "172.27.213.161/25" }
    node02 = { name = "node02", serial = 2002, mac = "06:00:00:00:20:02", ipv4_addr = "172.27.213.162/25" }
    node03 = { name = "node03", serial = 2003, mac = "06:00:00:00:20:03", ipv4_addr = "172.27.213.163/25" }
  }
  all_hosts = concat(
    [
      {
        name      = local.control_plane.name
        ipv4_addr = replace(local.control_plane.ipv4_addr, "/[/][0-9]{1,2}$/", "")
      }
    ],
    [
      for node in local.nodes : {
        name      = node.name
        ipv4_addr = replace(node.ipv4_addr, "/[/][0-9]{1,2}$/", "")
      }
    ]
  )
}

resource "zedamigo_vm" "CONTROL_PLANE_01" {
  name               = local.control_plane.name
  cpus               = 2
  mem                = "2G"
  serial_no          = local.control_plane.serial
  serial_port_server = true
  disk_image_base    = abspath(var.disk_image_base)
  disk_size_mb       = "100000" # ~ 100GB
  drive_if           = "virtio" # Required because the Debian `genericcloud` image doesn't have any drivers for IDE/SATA disks.

  extra_qemu_args = [
    "-drive", "file=${zedamigo_cloud_init_iso.CI_DATA_CP_01.filename},format=raw,if=virtio", # Same comment as for `drive_if` above.
    "-nic", "tap,id=usernet1,ifname=${zedamigo_tap.TAP_INTF_CP_01.name},mac=${local.control_plane.mac},script=no,downscript=no,model=virtio",
  ]
}

resource "zedamigo_vm" "NODES" {
  for_each = local.nodes

  name               = each.value.name
  cpus               = 1
  mem                = "1G"
  serial_no          = each.value.serial
  serial_port_server = true
  disk_image_base    = abspath(var.disk_image_base)
  disk_size_mb       = "100000" # ~ 100GB
  drive_if           = "virtio" # Required because the Debian `genericcloud` image doesn't have any drivers for IDE/SATA disks.

  extra_qemu_args = [
    "-drive", "file=${zedamigo_cloud_init_iso.CI_DATA_NODES[each.key].filename},format=raw,if=virtio", # Same comment as for `drive_if` above.
    "-nic", "tap,id=usernet1,ifname=${zedamigo_tap.TAP_INTFS[each.key].name},mac=${each.value.mac},script=no,downscript=no,model=virtio",
  ]
}

resource "random_password" "script_preffix" {
  length  = 10
  special = false # Keeps it simple for shell commands
  upper   = true
  lower   = true
  numeric = true
}

resource "random_password" "k3s_token" {
  length  = 40
  special = false # Keeps it simple for shell commands
  upper   = true
  lower   = true
  numeric = true
}

locals {
  cp_addr = replace(local.control_plane.ipv4_addr, "/[/][0-9]{1,2}$/", "")
}

# k3s master setup.
resource "null_resource" "k3s_install_on_master" {
  depends_on = [zedamigo_vm.CONTROL_PLANE_01]

  connection {
    type    = "ssh"
    host    = local.cp_addr
    user    = var.user
    timeout = "10m"
  }

  provisioner "file" {
    source      = "./scripts/install_k3s.sh"
    destination = "/tmp/${random_password.script_preffix.result}_install_k3s.sh"
  }

  provisioner "remote-exec" {
    inline = [
      "set -eu;",
      "##",
      "S=\"install_k3s.sh\";",
      "C=\"48fe6ec10517263cc69e1c924cf6b283c59a2b942b9b46186fc7c8d29e6f243a\";",
      "T=\"$(mktemp -d --suffix=\"${random_password.script_preffix.result}\" -p /tmp/ \"$S.XXXXXXXX\")\";",
      "##",
      "mv \"/tmp/${random_password.script_preffix.result}_$S\" \"$T/$S\";",
      "chmod 0700 \"$T/$S\";",
      "echo \"$C  $T/$S\" | sha256sum -c;",
      "##",
      "sleep \"$(( RANDOM % 10 ))\";",
      "until \"$T/$S\" server --token=\"${random_password.k3s_token.result}\" --bind-address \"${local.cp_addr}\" --advertise-address \"${local.cp_addr}\" --node-ip \"${local.cp_addr}\" --flannel-iface \"k3s-lab-net\" 1>\"$T/$S.out\" 2>\"$T/$S.err\"; do sleep \"$(( RANDOM % 7 ))\"; done;",
      "echo \"$?\" > \"$T/$S.rc\";",
    ]
  }
}

resource "null_resource" "get_kubectl_config" {
  depends_on = [null_resource.k3s_install_on_master]

  connection {
    type    = "ssh"
    host    = local.cp_addr
    user    = var.user
    timeout = "10m"
  }

  provisioner "local-exec" {
    command = "mkdir -p '.kube'; chmod 0700 '.kube'; ssh -o StrictHostKeyChecking=no ${var.user}@${local.cp_addr} 'sudo cat /etc/rancher/k3s/k3s.yaml' | tee '.kube/config'"
  }
}

# k3s nodes setup.
resource "null_resource" "k3s_install_on_nodes" {
  for_each = zedamigo_vm.NODES

  depends_on = [null_resource.k3s_install_on_master]

  connection {
    type    = "ssh"
    host    = replace(local.nodes[each.value.name].ipv4_addr, "/[/][0-9]{1,2}$/", "")
    user    = var.user
    timeout = "10m"
  }

  provisioner "file" {
    source      = "./scripts/install_k3s.sh"
    destination = "/tmp/${random_password.script_preffix.result}_install_k3s.sh"
  }

  provisioner "remote-exec" {
    inline = [
      "set -eu;",
      "##",
      "S=\"install_k3s.sh\";",
      "C=\"48fe6ec10517263cc69e1c924cf6b283c59a2b942b9b46186fc7c8d29e6f243a\";",
      "T=\"$(mktemp -d --suffix=\"${random_password.script_preffix.result}\" -p /tmp/ \"$S.XXXXXXXX\")\";",
      "##",
      "mv \"/tmp/${random_password.script_preffix.result}_$S\" \"$T/$S\";",
      "chmod 0700 \"$T/$S\";",
      "echo \"$C  $T/$S\" | sha256sum -c;",
      "##",
      "sleep \"$(( RANDOM % 60 ))\";",
      "export K3S_URL=\"https://master01:6443\";",
      "export K3S_TOKEN=\"${random_password.k3s_token.result}\";",
      "until \"$T/$S\" agent --node-ip \"${replace(local.nodes[each.value.name].ipv4_addr, "/[/][0-9]{1,2}$/", "")}\" --flannel-iface \"k3s-lab-net\" 1>\"$T/$S.out\" 2>\"$T/$S.err\"; do sleep \"$(( RANDOM % 15 ))\"; done",
      "echo \"$?\" > \"$T/$S.rc\";",
    ]
  }
}
