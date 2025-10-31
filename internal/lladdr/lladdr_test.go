package lladdr

import (
	"net"
	"testing"
)

func TestLinkLocalIPv6FromMAC_EUI48(t *testing.T) {
	ip, err := LinkLocalIPv6FromMACString("00:1c:42:2e:60:4a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "fe80::21c:42ff:fe2e:604a"
	if got := ip.String(); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestLinkLocalIPv6FromMAC_2ndTry(t *testing.T) {
	ip, err := LinkLocalIPv6FromMACString("22:83:37:a6:d1:98")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "fe80::2083:37ff:fea6:d198"
	if got := ip.String(); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestLinkLocalIPv6FromMAC_EUI64(t *testing.T) {
	hw := net.HardwareAddr{0x02, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77}
	ip, err := LinkLocalIPv6FromMAC(hw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "fe80::11:2233:4455:6677"
	if got := ip.String(); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestLinkLocalIPv6FromMAC_InvalidLength(t *testing.T) {
	// 5-byte MAC should be rejected.
	if _, err := LinkLocalIPv6FromMAC(net.HardwareAddr{0, 1, 2, 3, 4}); err == nil {
		t.Fatalf("expected error for invalid MAC length, got nil")
	}
}
