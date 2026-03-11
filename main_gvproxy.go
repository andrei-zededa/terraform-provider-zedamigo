// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/containers/gvisor-tap-vsock/pkg/transport"
	"github.com/containers/gvisor-tap-vsock/pkg/types"
	"github.com/containers/gvisor-tap-vsock/pkg/virtualnetwork"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

const (
	gvproxyDefaultSubnet     = "192.168.127.0/24"
	gvproxyDefaultGatewayIP  = "192.168.127.1"
	gvproxyDefaultGatewayMAC = "5a:94:ef:e4:0c:dd"
	gvproxyDefaultHostIP     = "192.168.127.254"
	gvproxyDefaultGuestIP    = "192.168.127.2"
	gvproxyDefaultGuestMAC   = "5a:94:ef:e4:0c:ee"
)

// parseForwards converts a comma-separated forwards string into a map.
// Format: "hostAddr:hostPort/guestAddr:guestPort,..."
// Example: "0.0.0.0:2222/192.168.127.2:22,0.0.0.0:2223/192.168.127.2:10022"
func parseForwards(raw string) (map[string]string, error) {
	if raw == "" {
		return nil, nil
	}
	result := make(map[string]string)
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		// Strip optional "tcp://" prefix for convenience.
		entry = strings.TrimPrefix(entry, "tcp://")

		parts := strings.SplitN(entry, "/", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid forward %q: expected hostAddr:port/guestAddr:port", entry)
		}
		host := strings.TrimSpace(parts[0])
		guest := strings.TrimSpace(parts[1])
		if host == "" || guest == "" {
			return nil, fmt.Errorf("invalid forward %q: empty host or guest", entry)
		}
		result[host] = guest
	}
	return result, nil
}

func gvproxyMain() {
	log.SetLevel(log.InfoLevel)
	log.SetOutput(os.Stderr)

	listenVfkit := *gvproxyListenVfkit
	listenQemu := *gvproxyListenQemu
	forwardsRaw := *gvproxyForwards

	if listenVfkit == "" && listenQemu == "" {
		fmt.Fprintf(os.Stderr, "Error: In 'gvproxy' mode MUST specify either `-gp.listen-vfkit` or `-gp.listen-qemu`.\n")
		os.Exit(1)
	}
	if listenVfkit != "" && listenQemu != "" {
		fmt.Fprintf(os.Stderr, "Error: In 'gvproxy' mode CANNOT specify both `-gp.listen-vfkit` and `-gp.listen-qemu`.\n")
		os.Exit(1)
	}

	forwards, err := parseForwards(forwardsRaw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to parse forwards: %v\n", err)
		os.Exit(1)
	}

	var protocol types.Protocol
	if listenVfkit != "" {
		protocol = types.VfkitProtocol
	} else {
		protocol = types.QemuProtocol
	}

	config := types.Configuration{
		MTU:               1500,
		Subnet:            gvproxyDefaultSubnet,
		GatewayIP:         gvproxyDefaultGatewayIP,
		GatewayMacAddress: gvproxyDefaultGatewayMAC,
		Protocol:          protocol,
		NAT: map[string]string{
			gvproxyDefaultHostIP: "127.0.0.1",
		},
		GatewayVirtualIPs: []string{gvproxyDefaultHostIP},
		DHCPStaticLeases: map[string]string{
			gvproxyDefaultGuestIP: gvproxyDefaultGuestMAC,
		},
		DNS: []types.Zone{
			{
				Name: "containers.internal.",
				Records: []types.Record{
					{Name: "gateway", IP: net.ParseIP(gvproxyDefaultGatewayIP)},
					{Name: "host", IP: net.ParseIP(gvproxyDefaultHostIP)},
				},
			},
		},
		Forwards: forwards,
	}

	vn, err := virtualnetwork.New(&config)
	if err != nil {
		log.Fatalf("Failed to create virtual network: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	groupErrs, ctx := errgroup.WithContext(ctx)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	groupErrs.Go(func() error {
		select {
		case <-sigChan:
			cancel()
			return errors.New("signal caught")
		case <-ctx.Done():
			return nil
		}
	})

	// Start the HTTP control endpoint inside the virtual network.
	ln, err := vn.Listen("tcp", fmt.Sprintf("%s:80", gvproxyDefaultGatewayIP))
	if err != nil {
		log.Fatalf("Failed to listen on gateway: %v", err)
	}
	mux := http.NewServeMux()
	mux.Handle("/services/forwarder/all", vn.Mux())
	mux.Handle("/services/forwarder/expose", vn.Mux())
	mux.Handle("/services/forwarder/unexpose", vn.Mux())
	groupErrs.Go(func() error {
		<-ctx.Done()
		return ln.Close()
	})
	groupErrs.Go(func() error {
		s := &http.Server{
			Handler:      mux,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
		}
		err := s.Serve(ln)
		if err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	})

	if listenVfkit != "" {
		conn, err := transport.ListenUnixgram(listenVfkit)
		if err != nil {
			log.Fatalf("vfkit listen error: %v", err)
		}

		groupErrs.Go(func() error {
			<-ctx.Done()
			if err := conn.Close(); err != nil {
				log.Errorf("error closing %s: %v", listenVfkit, err)
			}
			parsed, _ := url.Parse(listenVfkit)
			return os.Remove(parsed.Path)
		})

		groupErrs.Go(func() error {
			vfkitConn, err := transport.AcceptVfkit(conn)
			if err != nil {
				return fmt.Errorf("vfkit accept error: %w", err)
			}
			return vn.AcceptVfkit(ctx, vfkitConn)
		})

		log.Infof("gvproxy: listening vfkit %s", listenVfkit)
	}

	if listenQemu != "" {
		qemuListener, err := transport.Listen(listenQemu)
		if err != nil {
			log.Fatalf("qemu listen error: %v", err)
		}

		groupErrs.Go(func() error {
			<-ctx.Done()
			if err := qemuListener.Close(); err != nil {
				log.Errorf("error closing %s: %v", listenQemu, err)
			}
			parsed, _ := url.Parse(listenQemu)
			return os.Remove(parsed.Path)
		})

		groupErrs.Go(func() error {
			conn, err := qemuListener.Accept()
			if err != nil {
				return fmt.Errorf("qemu accept error: %w", err)
			}
			return vn.AcceptQemu(ctx, conn)
		})

		log.Infof("gvproxy: listening qemu %s", listenQemu)
	}

	if err := groupErrs.Wait(); err != nil {
		log.Errorf("gvproxy exiting: %v", err)
		os.Exit(1)
	}
}
