package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/socket"
)

func main() {
	var (
		listenPath  = flag.String("listen", "", "Listen on UNIX socket at given path")
		connectPath = flag.String("connect", "", "Connect to existing UNIX socket at given path")
		outputFile  = flag.String("out", "", "Output file (default: stdout)")
		verbose     = flag.Bool("v", false, "Verbose logging")
	)
	flag.Parse()

	// Set up a new logger.
	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	}))
	slog.SetDefault(logger)

	// Validate CLI flags.
	if *listenPath == "" && *connectPath == "" {
		fmt.Fprintf(os.Stderr, "Error: Must specify either -listen or -connect\n")
		flag.Usage()
		os.Exit(1)
	}

	if *listenPath != "" && *connectPath != "" {
		fmt.Fprintf(os.Stderr, "Error: Cannot specify both -listen and -connect\n")
		flag.Usage()
		os.Exit(1)
	}

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
