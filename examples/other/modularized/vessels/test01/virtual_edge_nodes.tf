#### This creates a QCOW2 disk image file which will be used for running the
#### QEMU VM with EVE-OS.
resource "zedamigo_disk_image" "empty_disk" {
  name    = "empty_disk_100GB"
  size_mb = 100000 # ~100GB
}

#### This creates a custom EVE-OS installer ISO, it basically runs
#### `docker run ... lfedge/eve installer_iso`.
resource "zedamigo_eve_installer" "eve_os_installer_iso" {
  name            = "EVE-OS_16.0.0-lts-kvm-amd64"
  tag             = "16.0.0-lts-kvm-amd64"
  cluster         = var.ZEDEDA_CLOUD_URL
  authorized_keys = var.nodes["DDDD"].ssh_pub_key
  grub_cfg        = <<-EOF
   set_getty
   # This is actually better for the QEMU VM case.
   set_global dom0_extra_args "$dom0_extra_args console=ttyS0 hv_console=ttyS0 dom0_console=ttyS0"
   EOF
}

#### This will start a QEMU VM with the EVE-OS installer ISO previously
#### created and run the install process.
resource "zedamigo_installed_edge_node" "ENODE_TEST_INSTALL_DDDD" {
  name = "ENODE_TEST_INSTALL_DDDD}"
  # This needs to match the serial no. of the edge-node object created in Zedcloud.
  serial_no       = var.nodes["DDDD"].serialno
  installer_iso   = zedamigo_eve_installer.eve_os_installer_iso.filename
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
resource "zedamigo_edge_node" "ENODE_TEST_VM_DDDD" {
  name = "ENODE_TEST_VM_DDDD"
  # Since this is just a test it doesn't needs to stricly match what is defined in the HW model.
  cpus               = 4
  mem                = "4G"
  serial_no          = zedamigo_installed_edge_node.ENODE_TEST_INSTALL_DDDD.serial_no
  serial_port_server = true
  disk_image_base    = zedamigo_installed_edge_node.ENODE_TEST_INSTALL_DDDD.disk_image
  ovmf_vars_src      = zedamigo_installed_edge_node.ENODE_TEST_INSTALL_DDDD.ovmf_vars
}
