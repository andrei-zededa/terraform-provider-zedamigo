// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/provider"
	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/socket"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
)

var (
	version   = "dev" // version string of the provider, should be set at build time.
	commit    = ""    // commit id, should be set at build time.
	buildDate = ""
	builtBy   = ""
	treeState = ""
)

var (
	debug = flag.Bool("debug", false, "Set to true to run the provider with support for debuggers like delve")

	showVersion = flag.Bool("version", false, "Show the provider version")

	socketTailer = flag.Bool("socket-tailer", false, "Run the binary in 'socket tailer' mode")

	listenPath  = flag.String("st.listen", "", "Socket tailer: listen on UNIX socket at given path")
	connectPath = flag.String("st.connect", "", "Socket tailer: connect to existing UNIX socket at given path")
	outputFile  = flag.String("st.out", "", "Socket tailer: output file (default: stdout)")
)

func main() {
	flag.Parse()

	if *showVersion {
		fmt.Fprintf(os.Stderr, "terraform-provider-zedamigo version '%s' (commit '%s', build date '%s', built by '%s', git tree state '%s').\n\n",
			version, commit, buildDate, builtBy, treeState)
		flag.Usage()
		os.Exit(0)
	}

	if *socketTailer {
		// Run in "socket tailer" mode and NOT the normal terraform provider mode.

		// Validate CLI flags.
		if *listenPath == "" && *connectPath == "" {
			fmt.Fprintf(os.Stderr, "Error: In 'socket tailer' mode MUST specify either `-st.listen` or `-st.connect`.\n")
			flag.Usage()
			os.Exit(1)
		}

		if *listenPath != "" && *connectPath != "" {
			fmt.Fprintf(os.Stderr, "Error: In 'socket tailer' mode CANNOT specify both `-st.listen` and `-st.connect` at the same time.\n")
			flag.Usage()
			os.Exit(1)
		}

		socketTailerMain()
		os.Exit(0)
	}

	opts := providerserver.ServeOpts{
		// TODO: Update this string with the published name of your provider.
		// Also update the tfplugindocs generate command to either remove the
		// -provider-name flag or set its value to the updated provider name.
		// Was:
		//   Address: "registry.terraform.io/hashicorp/scaffolding",
		// Probably this doesn't work as this isn't a terraform provider
		// registry.
		Address: "localhost/andrei-zededa/zedamigo",
		Debug:   *debug,
	}

	err := providerserver.Serve(context.Background(), provider.New(version), opts)
	if err != nil {
		log.Fatal(err.Error())
	}
}

func socketTailerMain() {
	// Set up a new logger.
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Determine mode and socket path.
	var mode, socketPath string
	if *listenPath != "" {
		mode = "listen"
		socketPath = *listenPath
	} else {
		mode = "connect"
		socketPath = *connectPath
	}

	var w io.Writer = os.Stdout
	var outputFileHandle *os.File
	if *outputFile != "" {
		file, err := os.OpenFile(*outputFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			logger.Error("Failed to open output file", "file", *outputFile, "error", err)
			os.Exit(1)
		}
		outputFileHandle = file
		w = file
		logger.Info("Writing to file", "file", *outputFile)
	} else {
		logger.Info("Writing to stdout")
	}

	tailer := socket.NewTailer(w, logger)

	// Set up a context for graceful shutdown.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		logger.Info("Received shutdown signal")
		cancel()
	}()

	// Run and handle cleanup
	var err error
	if mode == "listen" {
		err = tailer.RunServer(ctx, socketPath)
	} else {
		err = tailer.RunClient(ctx, socketPath)
	}

	// Cleanup
	if closeErr := tailer.Close(); closeErr != nil {
		logger.Error("Failed to close logger", "error", closeErr)
	}

	if outputFileHandle != nil {
		if closeErr := outputFileHandle.Close(); closeErr != nil {
			logger.Error("Failed to close output file", "error", closeErr)
		}
	}

	if err != nil {
		logger.Error("Operation failed", "mode", mode, "error", err)
		os.Exit(1)
	}
}
