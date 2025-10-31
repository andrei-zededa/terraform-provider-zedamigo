resource "zedamigo_bridge" "BRIDGE_101" {
  name         = "br101-${var.config_suffix}"
  mtu          = "1500"
  state        = "up"
  ipv4_address = "10.99.101.1/24"
  ipv6_address = "2001:db8:101:101::1/64"
}

resource "zedamigo_dhcp_server" "DHCP_101" {
  interface  = zedamigo_bridge.BRIDGE_101.name
  server_id  = "10.99.101.1"
  nameserver = "9.9.9.101"
  router     = "10.99.101.1"
  netmask    = "255.255.255.0"
  pool {
    start = "10.99.101.70"
    end   = "10.99.101.79"
  }
  lease_time = 86400
}

resource "zedamigo_radv" "SLAAC_101" {
  interface         = zedamigo_bridge.BRIDGE_101.name
  prefix            = "2001:db8:101:101::/64"
  dns_servers       = "2606:4700:4700::101:1111"
  prefix_autonomous = true  # Allow SLAAC.
  managed_config    = false # Don't require DHCPv6 for addresses.
  other_config      = false # Don't require DHCPv6 for other config.
  min_interval      = 20
  max_interval      = 60
}

resource "zedamigo_tap" "TAP_101" {
  name   = "tap101-${var.config_suffix}"
  mtu    = "1500"
  state  = "up"
  group  = "kvm"
  master = zedamigo_bridge.BRIDGE_101.name
}

resource "zedamigo_bridge" "BRIDGE_102" {
  name         = "br102-${var.config_suffix}"
  mtu          = "1500"
  state        = "up"
  ipv4_address = "10.99.102.1/24"
  ipv6_address = "2001:db8:102:102::1/64"
}

resource "zedamigo_dhcp_server" "DHCP_102" {
  interface  = zedamigo_bridge.BRIDGE_102.name
  server_id  = "10.99.102.1"
  nameserver = "9.9.9.102"
  router     = "10.99.102.1"
  netmask    = "255.255.255.0"
  pool {
    start = "10.99.102.80"
    end   = "10.99.102.89"
  }
  lease_time = 86400
}

resource "zedamigo_radv" "SLAAC_102" {
  interface         = zedamigo_bridge.BRIDGE_102.name
  prefix            = "2001:db8:102:102::/64"
  dns_servers       = "2606:4700:4700::102:1111"
  prefix_autonomous = true  # Allow SLAAC.
  managed_config    = false # Don't require DHCPv6 for addresses.
  other_config      = false # Don't require DHCPv6 for other config.
  min_interval      = 20
  max_interval      = 60
}

resource "zedamigo_tap" "TAP_102" {
  name   = "tap102-${var.config_suffix}"
  mtu    = "1500"
  state  = "up"
  group  = "kvm"
  master = zedamigo_bridge.BRIDGE_102.name
}

resource "zedamigo_bridge" "BRIDGE_103" {
  name         = "br103-${var.config_suffix}"
  mtu          = "1500"
  state        = "up"
  ipv4_address = "10.99.103.1/24"
  ipv6_address = "2001:db8:103:103::1/64"
}

resource "zedamigo_dhcp_server" "DHCP_103" {
  interface  = zedamigo_bridge.BRIDGE_103.name
  server_id  = "10.99.103.1"
  nameserver = "9.9.9.103"
  router     = "10.99.103.1"
  netmask    = "255.255.255.0"
  pool {
    start = "10.99.103.90"
    end   = "10.99.103.99"
  }
  lease_time = 86400
}

resource "zedamigo_radv" "SLAAC_103" {
  interface         = zedamigo_bridge.BRIDGE_103.name
  prefix            = "2001:db8:103:103::/64"
  dns_servers       = "2606:4700:4700::103:1111"
  prefix_autonomous = true  # Allow SLAAC.
  managed_config    = false # Don't require DHCPv6 for addresses.
  other_config      = false # Don't require DHCPv6 for other config.
  min_interval      = 20
  max_interval      = 60
}

resource "zedamigo_tap" "TAP_103" {
  name   = "tap103-${var.config_suffix}"
  mtu    = "1500"
  state  = "up"
  group  = "kvm"
  master = zedamigo_bridge.BRIDGE_103.name
}

resource "zedamigo_bridge" "BRIDGE_104" {
  name         = "br104-${var.config_suffix}"
  mtu          = "1500"
  state        = "up"
  ipv4_address = "10.99.104.1/24"
  ipv6_address = "2001:db8:104:104::1/64"
}
resource "zedamigo_dhcp_server" "DHCP_104" {
  interface  = zedamigo_bridge.BRIDGE_104.name
  server_id  = "10.99.104.1"
  nameserver = "9.9.9.104"
  router     = "10.99.104.1"
  netmask    = "255.255.255.0"
  pool {
    start = "10.99.104.100"
    end   = "10.99.104.109"
  }
  lease_time = 86400
}

resource "zedamigo_radv" "SLAAC_104" {
  interface         = zedamigo_bridge.BRIDGE_104.name
  prefix            = "2001:db8:104:104::/64"
  dns_servers       = "2606:4700:4700::104:1111"
  prefix_autonomous = true  # Allow SLAAC.
  managed_config    = false # Don't require DHCPv6 for addresses.
  other_config      = false # Don't require DHCPv6 for other config.
  min_interval      = 20
  max_interval      = 60
}

resource "zedamigo_tap" "TAP_104" {
  name   = "tap104-${var.config_suffix}"
  mtu    = "1500"
  state  = "up"
  group  = "kvm"
  master = zedamigo_bridge.BRIDGE_104.name
}

resource "zedamigo_bridge" "BRIDGE_105" {
  name         = "br105-${var.config_suffix}"
  mtu          = "1500"
  state        = "up"
  ipv4_address = "10.99.105.1/24"
  ipv6_address = "2001:db8:105:105::1/64"
}
resource "zedamigo_dhcp_server" "DHCP_105" {
  interface  = zedamigo_bridge.BRIDGE_105.name
  server_id  = "10.99.105.1"
  nameserver = "9.9.9.105"
  router     = "10.99.105.1"
  netmask    = "255.255.255.0"
  pool {
    start = "10.99.105.100"
    end   = "10.99.105.109"
  }
  lease_time = 86400
}

resource "zedamigo_radv" "MANAGED_105" {
  interface         = zedamigo_bridge.BRIDGE_105.name
  prefix            = "2001:db8:105:105::/64"
  dns_servers       = "2606:4700:4700::105:1111"
  prefix_autonomous = false # DO NOT allow SLAAC.
  managed_config    = true  # Require DHCPv6 for addresses.
  other_config      = true  # Require DHCPv6 for other config.
  min_interval      = 20
  max_interval      = 60
}

resource "zedamigo_dhcp6_server" "DHCPV6_105" {
  interface  = zedamigo_bridge.BRIDGE_105.name
  server_id  = zedamigo_bridge.BRIDGE_105.mac_address
  nameserver = "2606:4700:4700::105:1111"
  pool {
    start = "2001:db8:105:105::baad:0"
    end   = "2001:db8:105:105::baad:ff"
  }
}

resource "zedamigo_tap" "TAP_105" {
  name   = "tap105-${var.config_suffix}"
  mtu    = "1500"
  state  = "up"
  group  = "kvm"
  master = zedamigo_bridge.BRIDGE_105.name
}

resource "zedamigo_bridge" "BRIDGE_106" {
  name         = "br106-${var.config_suffix}"
  mtu          = "1500"
  state        = "up"
  ipv4_address = "10.99.106.1/24"
  ipv6_address = "2001:db8:106:106::1/64"
}
resource "zedamigo_dhcp_server" "DHCP_106" {
  interface  = zedamigo_bridge.BRIDGE_106.name
  server_id  = "10.99.106.1"
  nameserver = "9.9.9.106"
  router     = "10.99.106.1"
  netmask    = "255.255.255.0"
  pool {
    start = "10.99.106.100"
    end   = "10.99.106.109"
  }
  lease_time = 86400
}

resource "zedamigo_radv" "MANAGED_106" {
  interface         = zedamigo_bridge.BRIDGE_106.name
  prefix            = "2001:db8:106:106::/64"
  dns_servers       = "2606:4700:4700::106:1111"
  prefix_autonomous = false # DO NOT allow SLAAC.
  managed_config    = true  # Require DHCPv6 for addresses.
  other_config      = true  # Require DHCPv6 for other config.
  min_interval      = 20
  max_interval      = 60
}

resource "zedamigo_dhcp6_server" "DHCPV6_106" {
  interface  = zedamigo_bridge.BRIDGE_106.name
  server_id  = zedamigo_bridge.BRIDGE_106.mac_address
  nameserver = "2606:4700:4700::106:1111"
  pool {
    start = "2001:db8:106:106::baad:0"
    end   = "2001:db8:106:106::baad:ff"
  }
}

resource "zedamigo_tap" "TAP_106" {
  name   = "tap106-${var.config_suffix}"
  mtu    = "1500"
  state  = "up"
  group  = "kvm"
  master = zedamigo_bridge.BRIDGE_106.name
}

resource "zedamigo_tap" "TAP_107" {
  name  = "tap107-${var.config_suffix}"
  mtu   = "1500"
  state = "up"
  group = "kvm"
}

resource "zedamigo_tap" "TAP_108" {
  name  = "tap108-${var.config_suffix}"
  mtu   = "1500"
  state = "up"
  group = "kvm"
}

resource "zedamigo_vlan" "VLAN_208" {
  parent       = zedamigo_tap.TAP_108.name
  vlan_id      = 208
  mtu          = "1500"
  state        = "up"
  ipv4_address = "10.99.208.1/24"
  ipv6_address = "2001:db8:208:208::1/64"
}
resource "zedamigo_dhcp_server" "DHCP_208" {
  interface  = zedamigo_vlan.VLAN_208.name
  server_id  = "10.99.208.1"
  nameserver = "9.9.9.208"
  router     = "10.99.208.1"
  netmask    = "255.255.255.0"
  pool {
    start = "10.99.208.200"
    end   = "10.99.208.209"
  }
  lease_time = 86400
}

resource "zedamigo_radv" "MANAGED_208" {
  interface         = zedamigo_vlan.VLAN_208.name
  prefix            = "2001:db8:208:208::/64"
  prefix_autonomous = false # DO NOT allow SLAAC.
  managed_config    = true  # Require DHCPv6 for addresses.
  other_config      = true  # Require DHCPv6 for other config.
  min_interval      = 20
  max_interval      = 60
}

resource "zedamigo_dhcp6_server" "DHCPV6_208" {
  interface  = zedamigo_vlan.VLAN_208.name
  server_id  = zedamigo_vlan.VLAN_208.mac_address
  nameserver = "2606:4700:4700::208:1111"
  pool {
    start = "2001:db8:208:208::daaf:0"
    end   = "2001:db8:208:208::daaf:ff"
  }
}
