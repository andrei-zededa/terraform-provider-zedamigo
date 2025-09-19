resource "zedamigo_tap" "test-tap" {
  name  = "test-tap"
  mtu   = "1500"
  state = "up"
}

# NOTE: If the test-tap interface is not in use by a process then it's state
# will be `down`. This causes a failure when trying to set the state of the
# sub-interface to `up`. If it's parent interface would be something like `eth0`
# and that interface is `up` then we can also set the sub-interface state to `up`.
resource "zedamigo_vlan" "v2000" {
  parent       = "test-tap"
  vlan_id      = 2000
  mtu          = "1500"
  state        = "down" # Usually this would be set to `up`.
  ipv4_address = "172.27.203.225/27"
}
