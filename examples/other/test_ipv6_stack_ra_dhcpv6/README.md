# IPv6 with RA + DHCPv6

After the entire topology is created the VM should show the following:

```
lab@test-vm-01:~$ ip addr show
1: lo: <LOOPBACK,UP,LOWER_UP> mtu 65536 qdisc noqueue state UNKNOWN group default qlen 1000
    link/loopback 00:00:00:00:00:00 brd 00:00:00:00:00:00
    inet 127.0.0.1/8 scope host lo
       valid_lft forever preferred_lft forever
    inet6 ::1/128 scope host noprefixroute
       valid_lft forever preferred_lft forever
2: enp0s2: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc fq_codel state UP group default qlen 1000
    link/ether 52:54:00:12:34:56 brd ff:ff:ff:ff:ff:ff
    inet 10.0.2.15/24 metric 100 brd 10.0.2.255 scope global dynamic enp0s2
       valid_lft 86277sec preferred_lft 86277sec
    inet6 fe80::5054:ff:fe12:3456/64 scope link
       valid_lft forever preferred_lft forever
3: enp0s3: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc fq_codel state UP group default qlen 1000
    link/ether 8c:84:74:11:01:01 brd ff:ff:ff:ff:ff:ff
    inet6 2001:db8:113::baad:0/128 scope global dynamic noprefixroute
       valid_lft 172sec preferred_lft 172sec
    inet6 fe80::8e84:74ff:fe11:101/64 scope link
       valid_lft forever preferred_lft forever

lab@test-vm-01:~$ ip -6 route show
2001:db8:113::/64 dev enp0s3 proto ra metric 100 expires 86360sec hoplimit 64 pref medium
fe80::/64 dev enp0s2 proto kernel metric 256 pref medium
fe80::/64 dev enp0s3 proto kernel metric 256 pref medium
default via fe80::b4e4:fdff:fe3e:3e36 dev enp0s3 proto ra metric 100 expires 1760sec hoplimit 64 pref medium

lab@test-vm-01:~$ ping6 -c 5 2001:db8:113::1
PING 2001:db8:113::1 (2001:db8:113::1) 56 data bytes
64 bytes from 2001:db8:113::1: icmp_seq=1 ttl=64 time=2.05 ms
64 bytes from 2001:db8:113::1: icmp_seq=2 ttl=64 time=0.324 ms
64 bytes from 2001:db8:113::1: icmp_seq=3 ttl=64 time=0.390 ms
64 bytes from 2001:db8:113::1: icmp_seq=4 ttl=64 time=0.393 ms
64 bytes from 2001:db8:113::1: icmp_seq=5 ttl=64 time=0.423 ms

--- 2001:db8:113::1 ping statistics ---
5 packets transmitted, 5 received, 0% packet loss, time 4060ms
rtt min/avg/max/mdev = 0.324/0.715/2.046/0.666 ms

lab@test-vm-01:~$ ping6 -c 5 fe80::b4e4:fdff:fe3e:3e36%enp0s3
PING fe80::b4e4:fdff:fe3e:3e36%enp0s3 (fe80::b4e4:fdff:fe3e:3e36%enp0s3) 56 data bytes
64 bytes from fe80::b4e4:fdff:fe3e:3e36%enp0s3: icmp_seq=1 ttl=64 time=0.431 ms
64 bytes from fe80::b4e4:fdff:fe3e:3e36%enp0s3: icmp_seq=2 ttl=64 time=0.577 ms
64 bytes from fe80::b4e4:fdff:fe3e:3e36%enp0s3: icmp_seq=3 ttl=64 time=0.395 ms
64 bytes from fe80::b4e4:fdff:fe3e:3e36%enp0s3: icmp_seq=4 ttl=64 time=0.405 ms
64 bytes from fe80::b4e4:fdff:fe3e:3e36%enp0s3: icmp_seq=5 ttl=64 time=0.446 ms

--- fe80::b4e4:fdff:fe3e:3e36%enp0s3 ping statistics ---
5 packets transmitted, 5 received, 0% packet loss, time 4099ms
rtt min/avg/max/mdev = 0.395/0.450/0.577/0.065 ms

lab@test-vm-01:~$ resolvectl
Global
         Protocols: -LLMNR -mDNS -DNSOverTLS DNSSEC=no/unsupported
  resolv.conf mode: stub

Link 2 (enp0s2)
    Current Scopes: DNS
         Protocols: +DefaultRoute -LLMNR -mDNS -DNSOverTLS DNSSEC=no/unsupported
Current DNS Server: 10.0.2.3
       DNS Servers: 10.0.2.3

Link 3 (enp0s3)
    Current Scopes: DNS
         Protocols: +DefaultRoute -LLMNR -mDNS -DNSOverTLS DNSSEC=no/unsupported
Current DNS Server: 2606:4700:4700::1111
       DNS Servers: 2606:4700:4700::1111
```
