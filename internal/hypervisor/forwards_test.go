// SPDX-License-Identifier: MPL-2.0

package hypervisor

import (
	"testing"

	"github.com/matryer/is"
)

func TestStandardForwards(t *testing.T) {
	is := is.New(t)
	// The standard forwards are an invariant relied on by SLIRPNic0,
	// GvproxyForwards and DescribePortForwards; pin the count and contents so
	// the derived strings cannot silently drift.
	is.Equal(len(StandardForwards), 3)
	is.Equal(StandardForwards[0], StandardForward{HostOffset: 0, GuestPort: 22})
	is.Equal(StandardForwards[1], StandardForward{HostOffset: 1, GuestPort: 10022})
	is.Equal(StandardForwards[2], StandardForward{HostOffset: 2, GuestPort: 10080})
}

func TestSLIRPNic0(t *testing.T) {
	is := is.New(t)
	// Must reproduce the historical nic0Fmt output byte-for-byte.
	is.Equal(SLIRPNic0(50277),
		"user,id=usernet0,ipv6=off,hostfwd=tcp::50277-:22,hostfwd=tcp::50278-:10022,hostfwd=tcp::50279-:10080,model=virtio")
}

func TestSLIRPNic0Doc(t *testing.T) {
	is := is.New(t)
	is.Equal(SLIRPNic0Doc(),
		"user,id=usernet0,ipv6=off,hostfwd=tcp::<PORT>-:22,hostfwd=tcp::<PORT+1>-:10022,hostfwd=tcp::<PORT+2>-:10080,model=virtio")
}

func TestGvproxyForwards(t *testing.T) {
	is := is.New(t)
	// Must reproduce the historical inline gvproxy forwards from qemu.go / vfkit.go.
	is.Equal(GvproxyForwards(50277, "192.168.127.2"),
		"0.0.0.0:50277/192.168.127.2:22,0.0.0.0:50278/192.168.127.2:10022,0.0.0.0:50279/192.168.127.2:10080")
}

func TestDescribePortForwards(t *testing.T) {
	is := is.New(t)
	// localhost (and empty) render as 127.0.0.1.
	is.Equal(DescribePortForwards("localhost", 50277),
		"tcp/127.0.0.1:50277->:22, tcp/127.0.0.1:50278->:10022, tcp/127.0.0.1:50279->:10080")
	// A remote target renders its own address.
	is.Equal(DescribePortForwards("192.168.1.10", 50277),
		"tcp/192.168.1.10:50277->:22, tcp/192.168.1.10:50278->:10022, tcp/192.168.1.10:50279->:10080")
}
