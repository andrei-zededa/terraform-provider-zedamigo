# IPv6 with RA SLAAC

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
       valid_lft 86237sec preferred_lft 86237sec
    inet6 fe80::5054:ff:fe12:3456/64 scope link
       valid_lft forever preferred_lft forever
3: enp0s3: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc fq_codel state UP group default qlen 1000
    link/ether 8c:84:74:11:01:01 brd ff:ff:ff:ff:ff:ff
    inet6 2001:db8:113:0:8e84:74ff:fe11:101/64 scope global dynamic mngtmpaddr noprefixroute
       valid_lft 86390sec preferred_lft 14390sec
    inet6 fe80::8e84:74ff:fe11:101/64 scope link
       valid_lft forever preferred_lft forever

lab@test-vm-01:~$ ip -6 route show
2001:db8:113::/64 dev enp0s3 proto ra metric 100 expires 86382sec hoplimit 64 pref medium
fe80::/64 dev enp0s2 proto kernel metric 256 pref medium
fe80::/64 dev enp0s3 proto kernel metric 256 pref medium
default via fe80::b4e4:fdff:fe3e:3e36 dev enp0s3 proto ra metric 100 expires 1782sec hoplimit 64 pref medium

lab@test-vm-01:~$ ping6 -c 3 2001:db8:113::1
PING 2001:db8:113::1 (2001:db8:113::1) 56 data bytes
64 bytes from 2001:db8:113::1: icmp_seq=1 ttl=64 time=0.187 ms
64 bytes from 2001:db8:113::1: icmp_seq=2 ttl=64 time=0.273 ms
64 bytes from 2001:db8:113::1: icmp_seq=3 ttl=64 time=0.486 ms

--- 2001:db8:113::1 ping statistics ---
3 packets transmitted, 3 received, 0% packet loss, time 2035ms
rtt min/avg/max/mdev = 0.187/0.315/0.486/0.125 ms

lab@test-vm-01:~$ ping6 -c 3 fe80::8e84:74ff:fe11:101%enp0s3
PING fe80::8e84:74ff:fe11:101%enp0s3 (fe80::8e84:74ff:fe11:101%enp0s3) 56 data bytes
64 bytes from fe80::8e84:74ff:fe11:101%enp0s3: icmp_seq=1 ttl=64 time=0.021 ms
64 bytes from fe80::8e84:74ff:fe11:101%enp0s3: icmp_seq=2 ttl=64 time=0.038 ms
64 bytes from fe80::8e84:74ff:fe11:101%enp0s3: icmp_seq=3 ttl=64 time=0.051 ms

--- fe80::8e84:74ff:fe11:101%enp0s3 ping statistics ---
3 packets transmitted, 3 received, 0% packet loss, time 2034ms
rtt min/avg/max/mdev = 0.021/0.036/0.051/0.012 ms

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
       DNS Servers: 2606:4700:4700::1001 2606:4700:4700::1111
```
