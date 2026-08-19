#### The host side of the nine extra management ports of the edge-node.
####
#### Each port gets its own Linux network namespace on the host, its own TAP
#### interface inside that namespace and its own DHCP server bound to that TAP.
#### The TAP carries the gateway address of its subnet, so the default route a
#### port installs does resolve — and then stops there: a namespace holds
#### nothing but the TAP, so nothing forwards past the gateway. See the
#### "Ten management ports" section of the README for why that is the point.

# The nine extra management ports of the edge-node, eth1..eth9. This map is the
# single source for the four places which have to agree about them:
#
#   - `zedcloud_model.QEMU_VM` io_member_list (brands_and_models.tf) — Zedcloud
#     rejects an edge-node interface for a port the model does not declare;
#   - `zedcloud_edgenode.ENODE_TEST` interfaces (edge_nodes.tf);
#   - `zedamigo_edge_node.ENODE_TEST_VM` extra_qemu_args (edge_nodes.tf), the
#     QEMU NICs themselves;
#   - the netns / TAP / DHCP-server triplets in this file.
#
# Port ethN lives on 10.99.N.0/24 with the DHCP server — and the router it
# advertises — on 10.99.N.1.
locals {
  EXTRA_MGMT_PORTS = {
    for n in range(1, 10) : "eth${n}" => {
      index   = n
      subnet  = "10.99.${n}.0/24"
      gateway = "10.99.${n}.1"
      # The gateway in CIDR form. This is the address the host side of the
      # link — the TAP, once it is inside the namespace — is configured with,
      # and it is the same address the DHCP server advertises as the router.
      gateway_cidr = "10.99.${n}.1/24"
      netmask      = "255.255.255.0"
      # EVE-OS uses the cheapest usable management port first. eth0 — the QEMU
      # user-mode NIC, the only one that actually reaches the controller — is
      # cost 0, so these are strictly fallbacks and are not touched while eth0
      # is up. That matters for the downloader crash this example reproduces
      # (see the README): with all ports at equal cost EVE-OS would spread
      # download attempts across nine dead ends, making the 27 image pulls
      # slow and flaky, and the crashed node was downloading over a single
      # active uplink anyway.
      cost = n
    }
  }
}

resource "zedamigo_netns" "MGMT" {
  for_each = local.EXTRA_MGMT_PORTS

  name = "ns_${each.value.index}_${var.config_suffix}"
}

resource "zedamigo_tap" "MGMT" {
  for_each = local.EXTRA_MGMT_PORTS

  # Keep this short: an interface name is limited to 15 characters, and
  # `config_suffix` is part of it.
  name  = "tap${each.value.index}-${var.config_suffix}"
  mtu   = 1500
  state = "up"
  group = "kvm"
  netns = zedamigo_netns.MGMT[each.key].name
  # Applied by the provider inside the namespace, after the TAP is moved
  # there: a namespace move flushes the L3 addresses off a link, so this
  # cannot be configured up-front in the root namespace.
  ipv4_address = each.value.gateway_cidr
}

resource "zedamigo_dhcp_server" "MGMT" {
  for_each = local.EXTRA_MGMT_PORTS

  # A TAP is only moved into its namespace after the QEMU process has opened
  # it, so the DHCP server cannot bind to it before the VM is running.
  depends_on = [zedamigo_edge_node.ENODE_TEST_VM]

  interface  = zedamigo_tap.MGMT[each.key].name
  server_id  = each.value.gateway
  router     = each.value.gateway
  netmask    = each.value.netmask
  nameserver = "9.9.9.9"
  pool {
    start = cidrhost(each.value.subnet, 70)
    end   = cidrhost(each.value.subnet, 79)
  }
  lease_time = 86400
  netns      = zedamigo_netns.MGMT[each.key].name
}
