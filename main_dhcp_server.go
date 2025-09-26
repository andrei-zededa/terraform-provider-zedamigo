package main

import (
	"github.com/coredhcp/coredhcp/config"
	"github.com/coredhcp/coredhcp/logger"
	"github.com/coredhcp/coredhcp/server"

	"github.com/coredhcp/coredhcp/plugins"
	pl_autoconfigure "github.com/coredhcp/coredhcp/plugins/autoconfigure"
	pl_dns "github.com/coredhcp/coredhcp/plugins/dns"
	pl_file "github.com/coredhcp/coredhcp/plugins/file"
	pl_ipv6only "github.com/coredhcp/coredhcp/plugins/ipv6only"
	pl_leasetime "github.com/coredhcp/coredhcp/plugins/leasetime"
	pl_mtu "github.com/coredhcp/coredhcp/plugins/mtu"
	pl_nbp "github.com/coredhcp/coredhcp/plugins/nbp"
	pl_netmask "github.com/coredhcp/coredhcp/plugins/netmask"
	pl_prefix "github.com/coredhcp/coredhcp/plugins/prefix"
	pl_range "github.com/coredhcp/coredhcp/plugins/range"
	pl_router "github.com/coredhcp/coredhcp/plugins/router"
	pl_searchdomains "github.com/coredhcp/coredhcp/plugins/searchdomains"
	pl_serverid "github.com/coredhcp/coredhcp/plugins/serverid"
	pl_sleep "github.com/coredhcp/coredhcp/plugins/sleep"
	pl_staticroute "github.com/coredhcp/coredhcp/plugins/staticroute"

	"github.com/sirupsen/logrus"
)

var desiredPlugins = []*plugins.Plugin{
	&pl_autoconfigure.Plugin,
	&pl_dns.Plugin,
	&pl_file.Plugin,
	&pl_ipv6only.Plugin,
	&pl_leasetime.Plugin,
	&pl_mtu.Plugin,
	&pl_nbp.Plugin,
	&pl_netmask.Plugin,
	&pl_prefix.Plugin,
	&pl_range.Plugin,
	&pl_router.Plugin,
	&pl_searchdomains.Plugin,
	&pl_serverid.Plugin,
	&pl_sleep.Plugin,
	&pl_staticroute.Plugin,
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
