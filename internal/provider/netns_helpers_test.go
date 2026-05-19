// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/matryer/is"
)

func TestBuildIPCommand_NoSudo_NoNetns(t *testing.T) {
	is := is.New(t)
	conf := &ZedAmigoProviderConfig{
		IP:      "/usr/sbin/ip",
		UseSudo: false,
	}
	cmd, args := buildIPCommand(conf, "")
	is.Equal(cmd, "/usr/sbin/ip")
	is.Equal(len(args), 0)
}

func TestBuildIPCommand_Sudo_NoNetns(t *testing.T) {
	is := is.New(t)
	conf := &ZedAmigoProviderConfig{
		IP:      "/usr/sbin/ip",
		Sudo:    "/usr/bin/sudo",
		UseSudo: true,
	}
	cmd, args := buildIPCommand(conf, "")
	is.Equal(cmd, "/usr/bin/sudo")
	is.Equal(args, []string{"-n", "/usr/sbin/ip"})
}

func TestBuildIPCommand_NoSudo_WithNetns(t *testing.T) {
	is := is.New(t)
	conf := &ZedAmigoProviderConfig{
		IP:      "/usr/sbin/ip",
		UseSudo: false,
	}
	cmd, args := buildIPCommand(conf, "myns")
	is.Equal(cmd, "/usr/sbin/ip")
	is.Equal(args, []string{"netns", "exec", "myns", "/usr/sbin/ip"})
}

func TestBuildIPCommand_Sudo_WithNetns(t *testing.T) {
	is := is.New(t)
	conf := &ZedAmigoProviderConfig{
		IP:      "/usr/sbin/ip",
		Sudo:    "/usr/bin/sudo",
		UseSudo: true,
	}
	cmd, args := buildIPCommand(conf, "myns")
	is.Equal(cmd, "/usr/bin/sudo")
	is.Equal(args, []string{"-n", "/usr/sbin/ip", "netns", "exec", "myns", "/usr/sbin/ip"})
}
