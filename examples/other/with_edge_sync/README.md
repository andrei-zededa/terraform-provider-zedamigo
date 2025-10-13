# Topology with a VM running edge-sync

https://help.zededa.com/hc/en-us/articles/22651771053083-Configure-Edge-Synchronization#h_01K5CBE511QM6192YJ5HVH6S20

Once the *apply* is finished complete the topology by logging into the edge-sync
VM and creating the edge-sync container:

```
docker run -d --name edgesync -p 1180:1180 -p 8100:8100 zededa/edge-sync:latest
```

The edge-sync config for the edge-node must be added via the Zedcontrol WEB UI
as the zedcloud terraform provider doesn't currently support this.

If you want to use the local datastore to deploy an edge-app-instance while
the node is disconnected from Zedcloud you must have an Ubuntu disk image
(qcow2) in `./local_ds/images/` (see the config in `images.tf` and `datastores.tf`.

## How to disconnect the edge-node from Zedcloud

```
❯ tf state show 'zedamigo_edge_node.ENODE_TEST_VM["ENODE_TEST_AAAA"]'
# zedamigo_edge_node.ENODE_TEST_VM["ENODE_TEST_AAAA"]:
resource "zedamigo_edge_node" "ENODE_TEST_VM" {
    cpus               = "4"
    disk_image         = "/home/ev-zed1/.local/state/zedamigo/edge_nodes/71c99c0d-760d-4523-83c7-a7e52ccad1e5/disk0.disk_img.qcow2"
    disk_image_base    = "/home/ev-zed1/.local/state/zedamigo/installed_nodes/0bace5a6-e4e8-4f9f-a360-85dc156ee692/disk0.disk_img.qcow2"
    extra_qemu_args    = [
        "-nic",
        "tap,id=vmnet1,ifname=tap0-1287g,script=no,downscript=no,model=e1000,mac=8c:84:74:11:01:01",
    ]
    id                 = "71c99c0d-760d-4523-83c7-a7e52ccad1e5"
    mem                = "4G"
    name               = "ENODE_TEST_VM_AAAA_1287g"
    ovmf_vars          = "/home/ev-zed1/.local/state/zedamigo/edge_nodes/71c99c0d-760d-4523-83c7-a7e52ccad1e5/UEFI_OVMF_VARS.bin"
    ovmf_vars_src      = "/home/ev-zed1/.local/state/zedamigo/installed_nodes/0bace5a6-e4e8-4f9f-a360-85dc156ee692/UEFI_OVMF_VARS.bin"
    qmp_socket         = "unix:/home/ev-zed1/.local/state/zedamigo/edge_nodes/71c99c0d-760d-4523-83c7-a7e52ccad1e5/qmp.socket,server,nowait"
    serial_console_log = "/home/ev-zed1/.local/state/zedamigo/edge_nodes/71c99c0d-760d-4523-83c7-a7e52ccad1e5/serial_console_run.log"
    serial_no          = "SN_TEST_AAAA_1287g"
    serial_port_server = true
    serial_port_socket = "/home/ev-zed1/.local/state/zedamigo/edge_nodes/71c99c0d-760d-4523-83c7-a7e52ccad1e5/serial_port.socket"
    ssh_port           = 36267
    vm_running         = true
}

❯ socat UNIX-CONNECT:/home/ev-zed1/.local/state/zedamigo/edge_nodes/71c99c0d-760d-4523-83c7-a7e52ccad1e5/qmp.socket -
{"QMP": {"version": {"qemu": {"micro": 2, "minor": 0, "major": 10}, "package": ""}, "capabilities": ["oob"]}}

{ "execute": "qmp_capabilities" }
{"return": {}}

{ "execute": "human-monitor-command", "arguments": { "command-line": "set_link usernet0 off" } }
{"return": ""}

{ "execute": "human-monitor-command", "arguments": { "command-line": "set_link usernet0 on" } }
{"return": ""}
```
