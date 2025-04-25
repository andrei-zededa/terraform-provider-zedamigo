// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"flag"
	"log"

	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
)

var (
	version = "dev"
	commit  = ""
	date    = ""
)

func main() {
	var debug bool

	flag.BoolVar(&debug, "debug", false, "set to true to run the provider with support for debuggers like delve")
	flag.Parse()

	opts := providerserver.ServeOpts{
		// TODO: Update this string with the published name of your provider.
		// Also update the tfplugindocs generate command to either remove the
		// -provider-name flag or set its value to the updated provider name.
		// Was:
		//   Address: "registry.terraform.io/hashicorp/scaffolding",
		// Probably this doesn't work as this isn't a terraform provider
		// registry.
		Address: "localhost/andrei-zededa/zedamigo",
		Debug:   debug,
	}

	err := providerserver.Serve(context.Background(), provider.New(version), opts)
	if err != nil {
		log.Fatal(err.Error())
	}
}
