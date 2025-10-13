package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/andrei-zededa/hello-zedcloud/pkg/server"
)

func httpServerMain() ExitCode {
	// Prepare server configuration.
	config := server.Config{
		Listen:    *httpListen,
		StaticDir: *httpStaticDir,
		BwLimit:   *httpBwLimit,
		Username:  *httpUsername,
		Password:  *httpPassword,
		Version:   version,
	}

	// Create server instance.
	srv, err := server.New(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create HTTP server: %v", err)
		return ExitError
	}

	// Set up a context for graceful shutdown.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals for graceful shutdown.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start server in a goroutine.
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- srv.Serve()
	}()

	// Wait for either a signal or server error.
	select {
	case <-sigChan:
		if err := srv.Shutdown(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to shutdown server gracefully: %v", err)
			return ExitError
		}
	case err := <-serverErr:
		if err != nil {
			fmt.Fprintf(os.Stderr, "Server error: %v", err)
			return ExitError
		}
	}

	return ExitSuccess
}
