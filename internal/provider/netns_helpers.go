// SPDX-License-Identifier: MPL-2.0

package provider

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
