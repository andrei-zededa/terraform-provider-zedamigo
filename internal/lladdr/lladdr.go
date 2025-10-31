package lladdr

import (
	"fmt"
	"net"
)

// LinkLocalIPv6FromMAC returns the IPv6 link-local address (fe80::/64)
// derived from a 48-bit MAC (EUI-48) or 64-bit MAC (EUI-64).
// For EUI-48, it inserts ff:fe in the middle and flips the U/L bit.
func LinkLocalIPv6FromMAC(hw net.HardwareAddr) (net.IP, error) {
	if l := len(hw); l != 6 && l != 8 {
		return nil, fmt.Errorf("hardware address must be 6 or 8 bytes, got %d", l)
	}

	var iid [8]byte
	if len(hw) == 6 {
		// EUI-48 -> Modified EUI-64
		iid[0] = hw[0] ^ 0x02 // flip the U/L bit
		iid[1] = hw[1]
		iid[2] = hw[2]
		iid[3] = 0xff
		iid[4] = 0xfe
		iid[5] = hw[3]
		iid[6] = hw[4]
		iid[7] = hw[5]
	} else {
		// EUI-64 -> Modified EUI-64 (just flip U/L bit)
		copy(iid[:], hw[:8])
		iid[0] ^= 0x02
	}

	ip := net.IP(make([]byte, net.IPv6len))
	ip[0] = 0xfe
	ip[1] = 0x80
	// ip[2..7] remain zero
	copy(ip[8:], iid[:])
	return ip, nil
}

// LinkLocalIPv6FromMACString is a convenience wrapper useful when the MACi
// is a string.
func LinkLocalIPv6FromMACString(s string) (net.IP, error) {
	hw, err := net.ParseMAC(s)
	if err != nil {
		return nil, err
	}
	return LinkLocalIPv6FromMAC(hw)
}
