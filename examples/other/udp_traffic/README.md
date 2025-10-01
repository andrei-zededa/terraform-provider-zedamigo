# App instance traffic test topology 

This Terraform configuration creates a test topology that can be used for
testing various scenarios of traffic between the *outside* of an edge-node
and an app instance running on that node, including UDP traffic.

## Topology diagram

```
 ┌──────────────────────────────────────────────────────────────────────────────────────────────────────────────┐ 
 │                                       HOST                                                                   │ 
 │                                                                                                              │ 
 │        ┌───────────────┐              ┌──────────────────────────────────────────────────────────────────┐   │ 
 │        │ QEMU internal │              │                                                                  │   │ 
 │        │ DHCP server   ┼────────┐     │                      EDGE-NODE                                   │   │ 
 │        │   +NAT        │        │     │ 10.0.2.15                                                        │   │ 
 │        │ (10.0.2.0/24) │        │     │       ┌──────────────────┐                                       │   │ 
 │        └───────────────┘        │     │ keth0 │ network-instance │          ┌─────────────────────────┐  │   │ 
 │                                 └─────┼───────┤ type LOCAL       ┼─────┐    │     APP-INSTANCE VM     │  │   │ 
 │                                       │       │ port = Uplink    │     │    │        UBUNTU 24.04     │  │   │ 
 │           ┌───────────┐               │       └──────────────────┘     │    │                         │  │   │ 
 │           │   br101   │         tap101│ keth1                          │    │ enp3s0                  │  │   │ 
 │           │10.99.101.1┼───────────────┼───────                         └────┼─────                    │  │   │ 
 │           │ DHCPserver│               │                                     │                         │  │   │ 
 │           └───────────┘               │                                     │    (exact enpXs0 intf.  │  │   │ 
 │                                       │                                     │      might differ)      │  │   │ 
 │           ┌───────────┐               │       ┌──────────────────┐          │                         │  │   │ 
 │           │   br102   │         tap102│ keth2 │ network-instance │          │ enp4s0                  │  │   │ 
 │           │10.99.102.1┼───────────────┼───────┤ type LOCAL       ┼──────────┼─────                    │  │   │ 
 │           │ DHCPserver│               │       │ port = eth2      │          │                         │  │   │ 
 │           └───────────┘               │       └──────────────────┘          │                         │  │   │ 
 │                                       │                                     │                         │  │   │ 
 │           ┌───────────┐               │       ┌──────────────────┐          │                         │  │   │ 
 │           │   br103   │         tap103│ keth3 │ network-instance │          │ enp5s0                  │  │   │ 
 │           │10.99.103.1┼───────────────┼───────┤ type SWITCH      ┼──────────┼─────                    │  │   │ 
 │           │ DHCPserver│               │       │ port = eth3      │          │                         │  │   │ 
 │           └───────────┘               │       └──────────────────┘          └─────────────────────────┘  │   │ 
 │                                       │                                                                  │   │ 
 │                                       └──────────────────────────────────────────────────────────────────┘   │ 
 └──────────────────────────────────────────────────────────────────────────────────────────────────────────────┘ 
```

As it can be seen in diagram the configuration makes use of several *advanced*
zedamigo features to create an edge-node with 4 interfaces:

- The first interface (eth0) will be connected to the embedded QEMU DHCP server+NAT,
  meaning that it will have Internet access, if the host has that, without any
  firewall or other configuration on the host. However this means that the node
  is not directly accesible from the host on that interface. By default zedamigo
  creates several port fowards from localhost towards that "internal" QEMU IPv4
  address of the node:
  ```
  localhost:$random   -> 10.0.2.15:22
  localhost:$random+1 -> 10.0.2.15:10022
  localhost:$random+2 -> 10.0.2.15:10080
  ```
  The `$random` can be found in the zedamigo edge-node VM resource as `ssh_port`:
  ```
  ❯ tf state show zedamigo_edge_node.ENODE_TEST_VM_AAAA
    # zedamigo_edge_node.ENODE_TEST_VM_AAAA:
    resource "zedamigo_edge_node" "ENODE_TEST_VM_AAAA" {
        cpus               = "2"
        disk_image         = "/home/ev-zed1/.local/state/zedamigo/edge_nodes/b6074dc5-66dc-4666-b66e-493609f301ad/disk0.disk_img.qcow2"
        disk_image_base    = "/home/ev-zed1/.local/state/zedamigo/installed_nodes/de3c865d-fc34-47e6-8e25-9d740d2cb33f/disk0.disk_img.qcow2"
        extra_qemu_args    = [
            "-nic",
            "tap,id=vmnet1,ifname=tap101-1287g,script=no,downscript=no,model=e1000,mac=8c:84:74:11:01:01",
            "-nic",
            "tap,id=vmnet2,ifname=tap102-1287g,script=no,downscript=no,model=e1000,mac=8c:84:74:11:01:02",
            "-nic",
            "tap,id=vmnet3,ifname=tap103-1287g,script=no,downscript=no,model=e1000,mac=8c:84:74:11:01:03",
        ]
        id                 = "b6074dc5-66dc-4666-b66e-493609f301ad"
        mem                = "2G"
        name               = "ENODE_TEST_VM_AAAA_1287g"
        ovmf_vars          = "/home/ev-zed1/.local/state/zedamigo/edge_nodes/b6074dc5-66dc-4666-b66e-493609f301ad/UEFI_OVMF_VARS.bin"
        ovmf_vars_src      = "/home/ev-zed1/.local/state/zedamigo/installed_nodes/de3c865d-fc34-47e6-8e25-9d740d2cb33f/UEFI_OVMF_VARS.bin"
        qmp_socket         = "unix:/home/ev-zed1/.local/state/zedamigo/edge_nodes/b6074dc5-66dc-4666-b66e-493609f301ad/qmp.socket,server,nowait"
        serial_console_log = "/home/ev-zed1/.local/state/zedamigo/edge_nodes/b6074dc5-66dc-4666-b66e-493609f301ad/serial_console_run.log"
        serial_no          = "SN_TEST_AAAA_1287g"
        serial_port_server = true
        serial_port_socket = "/home/ev-zed1/.local/state/zedamigo/edge_nodes/b6074dc5-66dc-4666-b66e-493609f301ad/serial_port.socket"
        ssh_port           = 32076
        vm_running         = true
    }
  ```
  ```
  ❯ ps auxwww | egrep 'qemu.*b6074dc5-66dc-4666-b66e-493609f301ad'
  ........ -nic user,id=usernet0,hostfwd=tcp::32076-:22,hostfwd=tcp::32077-:10022,hostfwd=tcp::32078-:10080,model=virtio ........
  ```

- The other 3 interfaces are each connected to a different bridge which is
  created on the host as a result of a `zedamigo_bridge` resource. Additionally
  on those 3 bridges 3 DHCP servers are running as a result of `zedamigo_dhcp_server`
  resources.

## Testing

If `vars.tf` contains a valid SSH public key than that will be injected both
into the edge-node (initially from custom EVE-OS installer image but then also
through `debug.enable.ssh`) but also into the app instance through a cloud-init
config/app custom config. Therefore we can SSH both into the node and the app
instance:

```
❯ ssh -l root -p 32076 localhost
EVE is Edge Virtualization Engine

Take a look around and don't forget to use eve(1).
00f4eccf-9bec-4909-ad25-5f184667c285:~# eve version
14.5.1-lts-kvm-amd64
```

```
❯ ssh -l labuser -p 32077 localhost
..................................
labuser@ubuntuteston00f4eccf-9bec-4909-ad25-5f184667c285:~$ lsb_release -a
No LSB modules are available.
Distributor ID:	Ubuntu
Description:	Ubuntu 24.04.3 LTS
Release:	24.04
Codename:	noble
```

Since the 3rd interface of the app instance is connected to the switch network-instance
this means that the app instance should get an IPv4 address directly from the host and
be reachable:

```
```
