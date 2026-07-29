resource "zedamigo_edge_node" "example" {
  name            = "wait_until_example"
  serial_no       = "0123456789"
  disk_image_base = "/var/lib/zedamigo/disk_images/example.qcow2"
}

# Barrier: block until the edge node answers on its forwarded SSH port. The
# script is a single-shot probe — exit 0 means "ready", any non-zero exit means
# "not yet, try again after `interval`". The resource owns the retrying, so the
# script needs no loop, no deadline and no sleep.
#
# The probe runs ON the provider `target`, which is the whole point: the same
# configuration works unchanged whether `target` is "localhost" (the script runs
# locally) or a remote host (the script runs over SSH). `ssh_port` is a port
# forwarded on the machine that actually runs the edge node, so probing it from
# anywhere else — as a `local-exec` provisioner would — is wrong as soon as the
# target is remote.
resource "zedamigo_wait_until" "node_ssh_ready" {
  triggers = {
    # Changing any trigger value re-runs the barrier. Referencing the node's id
    # also orders this resource after it.
    node_id = zedamigo_edge_node.example.id
  }

  timeout  = "10m"
  interval = "10s"

  # Optional backstop against a single hung attempt. Note that abandoning an
  # attempt does not kill it, so prefer self-bounding probes (as below, with
  # ConnectTimeout) and treat this as a safety net.
  attempt_timeout = "30s"

  script = <<-EOT
    set -u

    if ssh -o StrictHostKeyChecking=no \
           -o UserKnownHostsFile=/dev/null \
           -o ConnectTimeout=5 \
           -o LogLevel=ERROR \
           -p ${zedamigo_edge_node.example.ssh_port} root@localhost \
           'eve version'; then
      exit 0
    fi

    echo "not up yet" >&2
    exit 1
  EOT
}

# The successful attempt's output is recorded, so a probe can double as a way to
# read a value off the target.
output "eve_version" {
  value = trimspace(zedamigo_wait_until.node_ssh_ready.stdout)
}

output "wait_stats" {
  value = {
    attempts = zedamigo_wait_until.node_ssh_ready.attempts
    elapsed  = zedamigo_wait_until.node_ssh_ready.elapsed
  }
}
