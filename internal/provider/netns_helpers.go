// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"regexp"
	"strings"
)

// linkFlagsRegex captures the flags section of an `ip link show` line, e.g.
// the "BROADCAST,MULTICAST,UP,LOWER_UP" part of
// "2: br0: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 ...".
var linkFlagsRegex = regexp.MustCompile(`<([^>]*)>`)

// linkFlagUp reports whether the administrative UP flag is present in the
// flags section of an `ip link show` first line. It checks the comma-separated
// flag tokens (so it matches "UP" but not, say, a substring elsewhere on the
// line). This reflects the administrative state we set with `ip link set up`,
// independent of the operational "state DOWN"/NO-CARRIER condition that
// persists until a process (e.g. QEMU) opens a TAP.
func linkFlagUp(line string) bool {
	m := linkFlagsRegex.FindStringSubmatch(line)
	if len(m) < 2 {
		return false
	}
	for _, f := range strings.Split(m[1], ",") {
		if f == "UP" {
			return true
		}
	}
	return false
}

// buildIPCommand returns the command and base arguments for running `ip`
// commands, optionally inside a network namespace and/or with sudo.
//
// The resulting prefix should be prepended to the actual ip sub-command
// arguments. For example, to run `ip link show br0` inside netns "myns"
// with sudo, the caller would do:
//
//	ipCmd, ipArgs := buildIPCommand(conf, "myns")
//	allArgs := append(ipArgs, "link", "show", "br0")
//	cmd.Run(d, ipCmd, allArgs...)
func buildIPCommand(conf *ZedAmigoProviderConfig, netns string) (string, []string) {
	ipCmd := conf.IP
	ipArgs := []string{}
	if conf.UseSudo {
		ipCmd = conf.Sudo
		ipArgs = []string{"-n", conf.IP}
	}
	if netns != "" {
		if conf.UseSudo {
			ipArgs = []string{"-n", conf.IP, "netns", "exec", netns, conf.IP}
		} else {
			ipArgs = []string{"netns", "exec", netns, conf.IP}
		}
	}
	return ipCmd, ipArgs
}
