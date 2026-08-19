output "RESOURCE_BUDGET" {
  description = "Total resources which the edge-app-instances of this example ask for on the single edge-node"
  value = {
    app_instances      = local.RESOURCE_BUDGET.app_instances
    vcpus              = local.RESOURCE_BUDGET.vcpus
    memory             = "${local.RESOURCE_BUDGET.memory_mb / 1024} GB"
    storage            = "${(local.RESOURCE_BUDGET.persist_mb + local.RESOURCE_BUDGET.dedicated_mb) / 1024} GB"
    persistent_volumes = "${local.RESOURCE_BUDGET.persist_mb / 1024} GB"
    dedicated_volumes  = "${local.RESOURCE_BUDGET.dedicated_mb / 1024} GB"
  }
}

output "WORKLOAD_BREAKDOWN" {
  description = "Per-role breakdown of the resource budget"
  value = {
    for role, spec in local.WORKLOAD_ROLES : role => {
      # One image version per replica, see `image_tags` in `workloads.tf`.
      images     = [for t in spec.image_tags : "${spec.image}:${t}"]
      replicas   = spec.replicas
      vcpus      = spec.replicas * spec.cpus
      memory_mb  = spec.replicas * spec.memory_mb
      persist_mb = spec.replicas * spec.persist_mb
      data_mb    = spec.replicas * spec.data_mb
    }
  }
}

output "POSTGRES_CREDENTIALS" {
  description = "The superuser credentials baked into the environment of every PostgreSQL app-instance"
  sensitive   = true
  value = {
    user     = "postgres"
    password = random_password.postgres_superuser.result
    database = local.WORKLOAD_ROLES.postgres_primary.env.POSTGRES_DB
  }
}

output "EDGE_APP_INSTANCES" {
  description = "The edge-app-instances which have been created, with the edge-node port each one is reachable on"
  value = {
    for name, w in local.WORKLOADS : name => {
      id        = zedcloud_application_instance.APP_INSTANCES[name].id
      vcpus     = w.cpus
      memory_mb = w.memory_mb
      # `<edge-node port> -> <container port>`, see the port map ACL in `edge_apps.tf`.
      portmap = "${w.node_port} -> ${w.app_port}"
    }
  }
}

output "EDGE_APP_LOCALHOST_PORT" {
  description = "zedamigo forwards this host port to edge-node port 10080, which is port-mapped to the first workload of the catalog (2 levels of port forwarding)"
  value = {
    localhost_port = zedamigo_edge_node.ENODE_TEST_VM.ssh_port + 2
    workload       = sort(keys(local.WORKLOADS))[0]
  }
}

output "EDGE_NODE_SSH_PORT" {
  description = "Localhost port forwarded to TCP port 22 of the edge-node (EVE-OS)"
  value       = zedamigo_edge_node.ENODE_TEST_VM.ssh_port
}

output "EXTRA_MGMT_PORTS" {
  description = "The nine extra management ports of the edge-node and the host network namespace each one is wired into"
  value = {
    for name, port in local.EXTRA_MGMT_PORTS : name => {
      netns      = zedamigo_netns.MGMT[name].name
      tap        = zedamigo_tap.MGMT[name].name
      subnet     = port.subnet
      gateway    = zedamigo_tap.MGMT[name].ipv4_address
      dhcp_range = "${cidrhost(port.subnet, 70)} - ${cidrhost(port.subnet, 79)}"
      cost       = port.cost
    }
  }
}

output "EDGE_NODE_QMP_SOCKET" {
  description = "Path, ON THE PROVIDER TARGET, of the QMP socket of the edge-node VM, for poking the VM by hand (e.g. `set_link` to simulate a carrier loss)"
  value       = local.QMP_SOCKET_PATH
}

output "CONTAINER_IMAGE_CATALOG" {
  description = "The 27 distinct container images the edge-node downloads — the unique-blob volume behind the downloader crash this example reproduces"
  value       = sort([for k in local.CONTAINER_IMAGE_SET : k])
}
