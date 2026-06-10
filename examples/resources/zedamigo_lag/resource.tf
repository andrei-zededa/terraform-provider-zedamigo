resource "zedamigo_lag" "bond0" {
  name             = "test-bond-0"
  mode             = "802.3ad"
  miimon           = 100
  lacp_rate        = "fast"
  xmit_hash_policy = "layer3+4"
  mtu              = 1500
  state            = "up"
  ipv4_address     = "172.27.200.1/24"

  # Existing interfaces to aggregate as members (slaves) of the bond. Each one
  # is brought down, enslaved (`ip link set dev <interface> master <bond>`) and
  # brought back up. The interfaces must already exist in the same network
  # namespace as the bond.
  #
  # Changing this set is applied in-place: members are added/removed without
  # re-creating the bond.
  #
  # Do NOT list interfaces that attach themselves via their own `master`
  # attribute (such as `zedamigo_tap` resources) — those are owned by the other
  # resource and are deliberately ignored here.
  enslaved_interfaces = ["eth1", "eth2"]
}
