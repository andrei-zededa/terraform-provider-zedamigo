# Host side of the link-aggregation group: a Linux bond in 802.3ad (LACP) mode,
# with the fast LACP rate. Its members are the two TAP interfaces below.
#
# NOTE on how the members attach: each `zedamigo_tap` attaches *itself* to this
# bond via its own `master` attribute (just like a TAP attaches to a bridge in
# the other examples). Because the TAPs own that relationship, the LAG does NOT
# list them in `enslaved_interfaces` — that attribute is only for interfaces the
# LAG itself should enslave, and listing self-attaching TAPs there would make
# the two resources fight over the same members.
resource "zedamigo_lag" "BOND_0" {
  name             = "lab-bond0"
  mode             = "802.3ad"
  miimon           = 100
  lacp_rate        = "fast"
  xmit_hash_policy = "layer3+4"
  mtu              = 1500
  state            = "up"
  ipv4_address     = "10.10.10.1/24"
}

# First bond member. The TAP is created down, attached to the bond (a bond slave
# must be down at the moment it is enslaved — which it is, since the TAP only
# goes `up` after `master` is set) and then brought up. QEMU opens it when the
# VM starts, at which point the link gains carrier and LACP negotiates.
resource "zedamigo_tap" "LAG_M0" {
  name   = "lab-lagm0"
  mtu    = 1500
  state  = "up"
  group  = "kvm"
  master = zedamigo_lag.BOND_0.name
}

# Second bond member.
resource "zedamigo_tap" "LAG_M1" {
  name   = "lab-lagm1"
  mtu    = 1500
  state  = "up"
  group  = "kvm"
  master = zedamigo_lag.BOND_0.name
}
