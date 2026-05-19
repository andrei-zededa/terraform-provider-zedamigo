resource "zedamigo_netns" "TEST_NS_A" {
  name = "TEST_NS_A"
}

resource "null_resource" "CLEANUP_IPERF_SERVERS" {
  triggers = {
    ns_id = zedamigo_netns.TEST_NS_A.id
  }

  provisioner "local-exec" {
    when       = destroy
    on_failure = continue
    # WARNING: Obviously this kills any and all iperf instances irrespective on the network namespace.
    command = "sudo pkill -f iperf || true"
  }
}

resource "zedamigo_bridge" "BRIDGE_A" {
  name         = "br1-${var.config_suffix}"
  mtu          = "1500"
  state        = "up"
  ipv4_address = "10.99.1.1/24"
  netns        = zedamigo_netns.TEST_NS_A.name
}

resource "null_resource" "BRIDGE_A_extra_addrs" {
  triggers = {
    bridge_id = zedamigo_bridge.BRIDGE_A.id
  }

  provisioner "local-exec" {
    command = <<EOT
      NS="${zedamigo_netns.TEST_NS_A.name}";
      BR="${zedamigo_bridge.BRIDGE_A.name}";
      sudo ip netns exec $NS ip addr add 10.99.1.11/24 dev $BR;
      sudo ip netns exec $NS ip addr add 10.99.1.12/24 dev $BR;
      sudo ip netns exec $NS ip addr add 10.99.1.13/24 dev $BR;
      sudo ip netns exec $NS ip addr add 10.99.1.14/24 dev $BR;
      sudo ip netns exec $NS ip route add 10.99.2.0/24 via 10.99.1.71;
    EOT
  }
}

resource "null_resource" "SERVER_IPERFS" {
  triggers = {
    bridge_id = zedamigo_bridge.BRIDGE_A.id
  }

  depends_on = [null_resource.BRIDGE_A_extra_addrs]

  provisioner "local-exec" {
    command = <<EOT
      NS="${zedamigo_netns.TEST_NS_A.name}";
      sudo ip netns exec $NS taskset -c "0" iperf -s -B 10.99.1.11 -u 2>s1.err 1>s1.out &
      sudo ip netns exec $NS taskset -c "1" iperf -s -B 10.99.1.12 -u 2>s1.err 1>s1.out &
      sudo ip netns exec $NS taskset -c "2" iperf -s -B 10.99.1.13 -u 2>s1.err 1>s1.out &
      sudo ip netns exec $NS taskset -c "3" iperf -s -B 10.99.1.14 -u 2>s1.err 1>s1.out &
    EOT
  }
}

resource "zedamigo_dhcp_server" "DHCP_A" {
  interface  = zedamigo_bridge.BRIDGE_A.name
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

resource "zedamigo_tap" "TAP_A_AAAA" {
  name   = "tapA-AA-${var.config_suffix}"
  mtu    = "1500"
  state  = "up"
  group  = "kvm"
  master = zedamigo_bridge.BRIDGE_A.name
  netns  = zedamigo_netns.TEST_NS_A.name
}

#? resource "zedamigo_tap" "TAP_A_BBBB" {
#?   name   = "tapA-BB-${var.config_suffix}"
#?   mtu    = "1500"
#?   state  = "up"
#?   group  = "kvm"
#?   master = zedamigo_bridge.BRIDGE_A.name
#?   netns  = zedamigo_netns.TEST_NS_A.name
#? }

resource "zedamigo_netns" "TEST_NS_B" {
  name = "TEST_NS_B"
}

resource "zedamigo_bridge" "BRIDGE_B" {
  name         = "br2-${var.config_suffix}"
  mtu          = "1500"
  state        = "up"
  ipv4_address = "10.99.2.1/24"
  netns        = zedamigo_netns.TEST_NS_B.name
}

resource "null_resource" "BRIDGE_B_extra_addrs" {
  triggers = {
    bridge_id = zedamigo_bridge.BRIDGE_B.id
  }

  provisioner "local-exec" {
    command = <<EOT
      NS="${zedamigo_netns.TEST_NS_B.name}";
      BR="${zedamigo_bridge.BRIDGE_B.name}";
      sudo ip netns exec $NS ip addr add 10.99.2.11/24 dev $BR;
      sudo ip netns exec $NS ip addr add 10.99.2.12/24 dev $BR;
      sudo ip netns exec $NS ip addr add 10.99.2.13/24 dev $BR;
      sudo ip netns exec $NS ip addr add 10.99.2.14/24 dev $BR;
      sudo ip netns exec $NS ip route add 10.99.1.0/24 via 10.99.2.71;
    EOT
  }
}

resource "null_resource" "CLIENT_IPERFS" {
  triggers = {
    bridge_id = zedamigo_bridge.BRIDGE_B.id
  }

  depends_on = [null_resource.BRIDGE_B_extra_addrs]

  provisioner "local-exec" {
    command = <<EOT
      NS="${zedamigo_netns.TEST_NS_B.name}";
      #### Just for reference for now.
      #### sudo ip netns exec $NS taskset -c 4 iperf -u -c 10.99.1.11 -b 10kpps -l 100 -t 1200 2>c1.err 1>c1.out &
      #### sudo ip netns exec $NS taskset -c 5 iperf -u -c 10.99.1.12 -b 10kpps -l 100 -t 1200 2>c2.err 1>c2.out &
      #### sudo ip netns exec $NS taskset -c 6 iperf -u -c 10.99.1.13 -b 10kpps -l 100 -t 1200 2>c3.err 1>c3.out &
      #### sudo ip netns exec $NS taskset -c 7 iperf -u -c 10.99.1.14 -b 10kpps -l 100 -t 1200 2>c4.err 1>c4.out &

    EOT
  }
}

resource "zedamigo_dhcp_server" "DHCP_B" {
  interface  = zedamigo_bridge.BRIDGE_B.name
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

resource "zedamigo_tap" "TAP_B_AAAA" {
  name   = "tapB-AA-${var.config_suffix}"
  mtu    = "1500"
  state  = "up"
  group  = "kvm"
  master = zedamigo_bridge.BRIDGE_B.name
  netns  = zedamigo_netns.TEST_NS_B.name
}

#? resource "zedamigo_tap" "TAP_B_BBBB" {
#?   name   = "tapB-BB-${var.config_suffix}"
#?   mtu    = "1500"
#?   state  = "up"
#?   group  = "kvm"
#?   master = zedamigo_bridge.BRIDGE_B.name
#?   netns  = zedamigo_netns.TEST_NS_B.name
#? }
