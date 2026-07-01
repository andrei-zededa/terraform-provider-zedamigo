// SPDX-License-Identifier: MPL-2.0

package hypervisor

import (
	"fmt"
	"strings"
)

// StandardForward describes one of the standard host->guest TCP port forwards
// configured for an edge node's first NIC (nic0). The host port is the base SSH
// port plus HostOffset; GuestPort is the port inside the guest (EVE-OS).
type StandardForward struct {
	HostOffset int
	GuestPort  int
}

// StandardForwards is the single source of truth for the default nic0 port
// forwards: sshPort->22 (SSH), sshPort+1->10022 and sshPort+2->10080 (commonly
// mapped to edge-app-instance ports). The SLIRP "-nic" string, the gvproxy
// "-gp.forwards" value and the human-readable description are all derived from
// this slice so they cannot drift apart.
var StandardForwards = []StandardForward{
	{HostOffset: 0, GuestPort: 22},
	{HostOffset: 1, GuestPort: 10022},
	{HostOffset: 2, GuestPort: 10080},
}

// SLIRPNic0 builds the default QEMU "-nic" user-mode networking string for the
// given base SSH port, including the StandardForwards host forwards. It is the
// single source for what was previously the nic0Fmt constant.
func SLIRPNic0(sshPort int32) string {
	parts := make([]string, 0, len(StandardForwards))
	for _, f := range StandardForwards {
		parts = append(parts, fmt.Sprintf("hostfwd=tcp::%d-:%d", int(sshPort)+f.HostOffset, f.GuestPort))
	}
	return "user,id=usernet0,ipv6=off," + strings.Join(parts, ",") + ",model=virtio"
}

// SLIRPNic0Doc renders the default nic0 string with symbolic host ports
// (<PORT>, <PORT+1>, ...) instead of concrete numbers, for use in the nic0
// attribute documentation. Kept in sync with SLIRPNic0 via StandardForwards.
func SLIRPNic0Doc() string {
	parts := make([]string, 0, len(StandardForwards))
	for _, f := range StandardForwards {
		host := "<PORT>"
		if f.HostOffset > 0 {
			host = fmt.Sprintf("<PORT+%d>", f.HostOffset)
		}
		parts = append(parts, fmt.Sprintf("hostfwd=tcp::%s-:%d", host, f.GuestPort))
	}
	return "user,id=usernet0,ipv6=off," + strings.Join(parts, ",") + ",model=virtio"
}

// GvproxyForwards builds the comma-separated value for gvproxy's "-gp.forwards"
// flag, mapping host 0.0.0.0:<port> to <guestIP>:<guestPort> for each standard
// forward derived from the base SSH port.
func GvproxyForwards(sshPort int32, guestIP string) string {
	parts := make([]string, 0, len(StandardForwards))
	for _, f := range StandardForwards {
		parts = append(parts, fmt.Sprintf("0.0.0.0:%d/%s:%d", int(sshPort)+f.HostOffset, guestIP, f.GuestPort))
	}
	return strings.Join(parts, ",")
}

// DescribePortForwards returns a human-readable, one-line description of the
// standard host->guest TCP port forwards configured for the default nic0, e.g.
// "tcp/127.0.0.1:50277->:22, tcp/127.0.0.1:50278->:10022, tcp/127.0.0.1:50279->:10080".
// host is the address at which the forwards are reachable: for a local target
// that is 127.0.0.1; for a remote (SSH) target the forwards bind on the remote
// host, so its address is shown (reach them at <host>:<port> from elsewhere, or
// tunnel with your own `ssh -L`). The guest IP is omitted because it differs by
// networking backend (SLIRP DHCP vs gvproxy 192.168.127.2) and is irrelevant
// from the host's point of view.
func DescribePortForwards(host string, sshPort int32) string {
	if host == "" || host == "localhost" {
		host = "127.0.0.1"
	}
	parts := make([]string, 0, len(StandardForwards))
	for _, f := range StandardForwards {
		parts = append(parts, fmt.Sprintf("tcp/%s:%d->:%d", host, int(sshPort)+f.HostOffset, f.GuestPort))
	}
	return strings.Join(parts, ", ")
}
