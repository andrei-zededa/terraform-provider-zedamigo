//go:build linux && amd64
// +build linux,amd64

package main

import (
	"github.com/coredhcp/coredhcp/config"
	"github.com/coredhcp/coredhcp/logger"
	"github.com/coredhcp/coredhcp/server"

	"github.com/coredhcp/coredhcp/plugins"
	pl_dns "github.com/coredhcp/coredhcp/plugins/dns"
	pl_file "github.com/coredhcp/coredhcp/plugins/file"
	pl_leasetime "github.com/coredhcp/coredhcp/plugins/leasetime"
	pl_prefix "github.com/coredhcp/coredhcp/plugins/prefix"
	pl_range "github.com/coredhcp/coredhcp/plugins/range"
	pl_searchdomains "github.com/coredhcp/coredhcp/plugins/searchdomains"
	pl_serverid "github.com/coredhcp/coredhcp/plugins/serverid"

	"github.com/sirupsen/logrus"
)

var desiredPlugins6 = []*plugins.Plugin{
	&pl_dns.Plugin,
	&pl_file.Plugin,
	&pl_leasetime.Plugin,
	&pl_prefix.Plugin,
	&pl_range.Plugin,
	&pl_searchdomains.Plugin,
	&pl_serverid.Plugin,
}

func dhcp6ServerMain() {
	log := logger.GetLogger("main")
	log.Logger.SetLevel(logrus.DebugLevel)

	cnf, err := config.Load(*dhcp6Config)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}
	// register plugins
	for _, plugin := range desiredPlugins6 {
		if err := plugins.RegisterPlugin(plugin); err != nil {
			log.Fatalf("Failed to register plugin '%s': %v", plugin.Name, err)
		}
	}

	// start server
	srv, err := server.Start(cnf)
	if err != nil {
		log.Fatal(err)
	}
	if err := srv.Wait(); err != nil {
		log.Error(err)
	}
}
