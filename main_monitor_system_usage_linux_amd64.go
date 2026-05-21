//go:build linux && amd64
// +build linux,amd64

package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/andrei-zededa/monitor-system-usage/pkg/msucollect"
	"github.com/andrei-zededa/monitor-system-usage/pkg/msuformat"
	"gopkg.in/yaml.v3"
)

// msuCfg is the YAML configuration for the monitor-system-usage mode.
type msuCfg struct {
	OutputFile  string        `yaml:"output_file"`
	Interval    time.Duration `yaml:"interval"`
	FlushEveryN int           `yaml:"flush_every_n"`
	Namespaces  []string      `yaml:"namespaces"`
	IncludeEnv  string        `yaml:"include_env"`
}

func monitorSystemUsageMain() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	cfgData, err := os.ReadFile(*msuConfig)
	if err != nil {
		logger.Error("Failed to read config file", "path", *msuConfig, "error", err)
		os.Exit(1)
	}

	var cfg msuCfg
	if err := yaml.Unmarshal(cfgData, &cfg); err != nil {
		logger.Error("Failed to parse config file", "error", err)
		os.Exit(1)
	}

	if cfg.OutputFile == "" {
		logger.Error("output_file is required in config")
		os.Exit(1)
	}
	if cfg.Interval <= 0 {
		cfg.Interval = 10 * time.Second
	}
	if cfg.FlushEveryN < 1 {
		cfg.FlushEveryN = 6
	}
	if cfg.IncludeEnv == "" {
		cfg.IncludeEnv = msuformat.EnvModeFiltered
	}

	w, err := msuformat.NewFileWriter(cfg.OutputFile)
	if err != nil {
		logger.Error("Failed to open MSU output file", "path", cfg.OutputFile, "error", err)
		os.Exit(1)
	}

	logger.Info("Monitor system usage started",
		"output", cfg.OutputFile,
		"interval", cfg.Interval,
		"flush_every_n", cfg.FlushEveryN,
		"namespaces", cfg.Namespaces,
		"include_env", cfg.IncludeEnv,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case sig := <-sigChan:
			logger.Info("Received signal, shutting down", "signal", sig)
			cancel()
		case <-ctx.Done():
		}
	}()

	runErr := msucollect.Run(ctx, msucollect.Config{
		Writer:      w,
		Interval:    cfg.Interval,
		FlushEveryN: cfg.FlushEveryN,
		Namespaces:  cfg.Namespaces,
		IncludeEnv:  cfg.IncludeEnv,
		CmdLine:     os.Args,
		Version:     "msu-collect-embedded-" + version,
	})

	if closeErr := w.Close(); closeErr != nil {
		logger.Warn("MSU close failed", "error", closeErr)
	}

	if runErr != nil {
		logger.Error("msucollect.Run failed", "error", runErr)
		os.Exit(1)
	}
}
