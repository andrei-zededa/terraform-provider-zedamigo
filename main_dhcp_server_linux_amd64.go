//go:build linux && amd64
// +build linux,amd64

package main

import (
	"github.com/coredhcp/coredhcp/config"
	"github.com/coredhcp/coredhcp/logger"
	"github.com/coredhcp/coredhcp/server"

	"github.com/coredhcp/coredhcp/plugins"
	pl_dns "github.com/coredhcp/coredhcp/plugins/dns"
	pl_netmask "github.com/coredhcp/coredhcp/plugins/netmask"
	pl_range "github.com/coredhcp/coredhcp/plugins/range"
	pl_router "github.com/coredhcp/coredhcp/plugins/router"
	pl_serverid "github.com/coredhcp/coredhcp/plugins/serverid"

	"github.com/sirupsen/logrus"
)

var desiredPlugins = []*plugins.Plugin{
	&pl_serverid.Plugin,
	&pl_dns.Plugin,
	&pl_router.Plugin,
	&pl_netmask.Plugin,
	&pl_range.Plugin,
}

func dhcpServerMain() {
	log := logger.GetLogger("main")
	log.Logger.SetLevel(logrus.DebugLevel)

	cnf, err := config.Load(*dhcpConfig)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}
	// register plugins
	for _, plugin := range desiredPlugins {
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
