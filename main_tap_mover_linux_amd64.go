//go:build linux && amd64
// +build linux,amd64

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/vishvananda/netlink"
	"gopkg.in/yaml.v3"
)

// tapMoverConfig is the YAML configuration for the TAP mover daemon.
type tapMoverCfg struct {
	TapName        string `yaml:"tap_name"`
	NetNS          string `yaml:"netns"`
	Master         string `yaml:"master"`
	State          string `yaml:"state"`
	StatusFile     string `yaml:"status_file"`
	PollIntervalMs int    `yaml:"poll_interval_ms"`
	TimeoutS       int    `yaml:"timeout_s"`
	UseSudo        bool   `yaml:"use_sudo"`
	SudoPath       string `yaml:"sudo_path"`
	IPPath         string `yaml:"ip_path"`
}

type tapMoverStatus struct {
	Status       string `json:"status"`
	Timestamp    string `json:"timestamp"`
	OperstateWas string `json:"operstate_was,omitempty"`
	Error        string `json:"error,omitempty"`
}

func tapMoverMain() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Read config file.
	cfgData, err := os.ReadFile(*tapMoverConfig)
	if err != nil {
		logger.Error("Failed to read config file", "path", *tapMoverConfig, "error", err)
		os.Exit(1)
	}

	var cfg tapMoverCfg
	if err := yaml.Unmarshal(cfgData, &cfg); err != nil {
		logger.Error("Failed to parse config file", "error", err)
		os.Exit(1)
	}

	if cfg.PollIntervalMs <= 0 {
		cfg.PollIntervalMs = 500
	}
	if cfg.TimeoutS <= 0 {
		cfg.TimeoutS = 300
	}

	logger.Info("TAP mover started",
		"tap", cfg.TapName,
		"netns", cfg.NetNS,
		"master", cfg.Master,
		"state", cfg.State,
		"timeout_s", cfg.TimeoutS,
	)

	timeout := time.Duration(cfg.TimeoutS) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Signal handling: cancel context on SIGINT/SIGTERM.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case sig := <-sigChan:
			logger.Info("Received signal, cancelling", "signal", sig)
			cancel()
		case <-ctx.Done():
		}
	}()

	// Subscribe to netlink link events.
	updates := make(chan netlink.LinkUpdate)
	done := make(chan struct{})
	defer close(done)
	if err := netlink.LinkSubscribe(updates, done); err != nil {
		errMsg := fmt.Sprintf("failed to subscribe to netlink events: %v", err)
		logger.Error(errMsg)
		writeStatus(cfg.StatusFile, tapMoverStatus{
			Status:    "error",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Error:     errMsg,
		})
		os.Exit(1)
	}

	logger.Info("Waiting for link event on TAP", "tap", cfg.TapName)

	for {
		select {
		case update := <-updates:
			if update.Attrs().Name != cfg.TapName {
				continue
			}
			operstate := update.Attrs().OperState.String()
			logger.Info("Link event for TAP detected",
				"tap", cfg.TapName,
				"operstate", operstate,
			)
			moveTAP(cfg, operstate, logger)
			return
		case <-ctx.Done():
			if ctx.Err() == context.DeadlineExceeded {
				logger.Error("Timeout waiting for TAP link event", "tap", cfg.TapName)
				writeStatus(cfg.StatusFile, tapMoverStatus{
					Status:    "error",
					Timestamp: time.Now().UTC().Format(time.RFC3339),
					Error:     fmt.Sprintf("timeout after %ds", cfg.TimeoutS),
				})
			} else {
				writeStatus(cfg.StatusFile, tapMoverStatus{
					Status:    "error",
					Timestamp: time.Now().UTC().Format(time.RFC3339),
					Error:     "cancelled by signal",
				})
			}
			os.Exit(1)
		}
	}
}

// moveTAP moves the TAP interface into the network namespace and configures it.
func moveTAP(cfg tapMoverCfg, operstate string, logger *slog.Logger) {
	// Move the TAP into the network namespace.
	if err := runIP(cfg, "link", "set", cfg.TapName, "netns", cfg.NetNS); err != nil {
		errMsg := fmt.Sprintf("failed to move TAP to netns: %v", err)
		logger.Error(errMsg)
		writeStatus(cfg.StatusFile, tapMoverStatus{
			Status:    "error",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Error:     errMsg,
		})
		os.Exit(1)
	}

	// Attach to bridge inside the netns (if master is specified).
	if cfg.Master != "" {
		if err := runIPInNetns(cfg, "link", "set", cfg.TapName, "master", cfg.Master); err != nil {
			errMsg := fmt.Sprintf("failed to set master on TAP inside netns: %v", err)
			logger.Error(errMsg)
			writeStatus(cfg.StatusFile, tapMoverStatus{
				Status:    "error",
				Timestamp: time.Now().UTC().Format(time.RFC3339),
				Error:     errMsg,
			})
			os.Exit(1)
		}
	}

	// Set state inside the netns (if specified).
	if cfg.State != "" {
		if err := runIPInNetns(cfg, "link", "set", cfg.TapName, cfg.State); err != nil {
			errMsg := fmt.Sprintf("failed to set state on TAP inside netns: %v", err)
			logger.Error(errMsg)
			writeStatus(cfg.StatusFile, tapMoverStatus{
				Status:    "error",
				Timestamp: time.Now().UTC().Format(time.RFC3339),
				Error:     errMsg,
			})
			os.Exit(1)
		}
	}

	logger.Info("TAP successfully moved and configured",
		"tap", cfg.TapName,
		"netns", cfg.NetNS,
	)

	writeStatus(cfg.StatusFile, tapMoverStatus{
		Status:       "moved",
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		OperstateWas: operstate,
	})
}

// runIP runs an ip command in the default namespace.
func runIP(cfg tapMoverCfg, args ...string) error {
	if cfg.UseSudo {
		allArgs := append([]string{"-n", cfg.IPPath}, args...)
		return execCmd(cfg.SudoPath, allArgs...)
	}
	return execCmd(cfg.IPPath, args...)
}

// runIPInNetns runs an ip command inside the configured network namespace.
func runIPInNetns(cfg tapMoverCfg, args ...string) error {
	if cfg.UseSudo {
		allArgs := append([]string{"-n", cfg.IPPath, "netns", "exec", cfg.NetNS, cfg.IPPath}, args...)
		return execCmd(cfg.SudoPath, allArgs...)
	}
	allArgs := append([]string{"netns", "exec", cfg.NetNS, cfg.IPPath}, args...)
	return execCmd(cfg.IPPath, allArgs...)
}

func execCmd(command string, args ...string) error {
	cmd := exec.Command(command, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %v: %w: %s", command, args, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func writeStatus(path string, status tapMoverStatus) {
	data, err := json.Marshal(status)
	if err != nil {
		slog.Error("Failed to marshal status", "error", err)
		return
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		slog.Error("Failed to write status file", "path", path, "error", err)
	}
}
