resource "zedamigo_netns" "TEST_NS_A" {
  name = "TEST_NS_A"
}

resource "zedamigo_dhcp_server" "DHCP_A" {
  depends_on = [zedamigo_edge_node.ENODE_TEST_VM_AAAA] # Because the TAPs are only moved inside the NS after the QEMU process starts.

  interface  = zedamigo_tap.TAP_AAAA_1.name
  server_id  = "10.99.1.1"
  nameserver = "9.9.9.9"
  router     = "10.99.1.1"
  netmask    = "255.255.255.0"
  pool {
    start = "10.99.1.70"
    end   = "10.99.1.79"
  }
  lease_time = 86400
  netns      = zedamigo_netns.TEST_NS_A.name
}

resource "zedamigo_tap" "TAP_AAAA_1" {
  name   = "tap1-AA-${var.config_suffix}"
  mtu    = "1500"
  state  = "up"
  group  = "kvm"
  netns  = zedamigo_netns.TEST_NS_A.name
}

resource "zedamigo_netns" "TEST_NS_B" {
  name = "TEST_NS_B"
}

resource "zedamigo_dhcp_server" "DHCP_B" {
  depends_on = [zedamigo_edge_node.ENODE_TEST_VM_AAAA] # Because the TAPs are only moved inside the NS after the QEMU process starts.

  interface  = zedamigo_tap.TAP_AAAA_2.name
  server_id  = "10.99.2.1"
  nameserver = "9.9.9.9"
  router     = "10.99.2.1"
  netmask    = "255.255.255.0"
  pool {
    start = "10.99.2.70"
    end   = "10.99.2.79"
  }
  lease_time = 86400
  netns      = zedamigo_netns.TEST_NS_B.name
}

resource "zedamigo_tap" "TAP_AAAA_2" {
  name   = "tap2-AA-${var.config_suffix}"
  mtu    = "1500"
  state  = "up"
  group  = "kvm"
  netns  = zedamigo_netns.TEST_NS_B.name
}

resource "zedamigo_netns" "TEST_NS_C" {
  name = "TEST_NS_C"
}

resource "zedamigo_dhcp_server" "DHCP_C" {
  depends_on = [zedamigo_edge_node.ENODE_TEST_VM_AAAA] # Because the TAPs are only moved inside the NS after the QEMU process starts.

  interface  = zedamigo_tap.TAP_AAAA_3.name
  server_id  = "10.99.3.1"
  nameserver = "9.9.9.9"
  router     = "10.99.3.1"
  netmask    = "255.255.255.0"
  pool {
    start = "10.99.3.70"
    end   = "10.99.3.79"
  }
  lease_time = 86400
  netns      = zedamigo_netns.TEST_NS_C.name
}

resource "zedamigo_tap" "TAP_AAAA_3" {
  name   = "tap3-AA-${var.config_suffix}"
  mtu    = "1500"
  state  = "up"
  group  = "kvm"
  netns  = zedamigo_netns.TEST_NS_C.name
}
