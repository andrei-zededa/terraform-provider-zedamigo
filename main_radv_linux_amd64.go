//go:build linux && amd64
// +build linux,amd64

package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/netip"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/mdlayher/ndp"
	"gopkg.in/yaml.v3"
)

type radvDaemonConfig struct {
	Interface               string `yaml:"interface"`
	Prefix                  string `yaml:"prefix"`
	PrefixOnLink            bool   `yaml:"prefix_on_link"`
	PrefixAutonomous        bool   `yaml:"prefix_autonomous"`
	PrefixValidLifetime     int64  `yaml:"prefix_valid_lifetime"`
	PrefixPreferredLifetime int64  `yaml:"prefix_preferred_lifetime"`
	DNSServers              string `yaml:"dns_servers"`
	ManagedConfig           bool   `yaml:"managed_config"`
	OtherConfig             bool   `yaml:"other_config"`
	RouterLifetime          int64  `yaml:"router_lifetime"`
	MaxInterval             int64  `yaml:"max_interval"`
	MinInterval             int64  `yaml:"min_interval"`
	HopLimit                int64  `yaml:"hop_limit"`
}

func radvMain() {
	if *radvConfig == "" {
		fmt.Fprintf(os.Stderr, "Error: -radv.config flag is required\n")
		os.Exit(1)
	}

	// Load configuration.
	configData, err := os.ReadFile(*radvConfig)
	if err != nil {
		log.Fatalf("Failed to read config file: %v", err)
	}

	config := radvDaemonConfig{}
	if err := yaml.Unmarshal(configData, &config); err != nil {
		log.Fatalf("Failed to parse config file: %v", err)
	}

	// Parse prefix.
	prefix, err := netip.ParsePrefix(config.Prefix)
	if err != nil {
		log.Fatalf("Invalid prefix %s: %v", config.Prefix, err)
	}

	// Build Router Advertisement message.
	ra := buildRouterAdvertisement(&config, prefix)

	// Create context for graceful shutdown.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Received shutdown signal")
		cancel()
	}()

	log.Printf("Starting RADV daemon on interface %s", config.Interface)

	iface, err := net.InterfaceByName(config.Interface)
	if err != nil {
		log.Fatalf("Failed to get interface %s: %v", config.Interface, err)
	}
	conn, ip, err := ndp.Listen(iface, ndp.LinkLocal)
	if err != nil {
		if !*radvWait {
			log.Fatalf("Failed to create NDP connection: %v", err)
		}
		for {
			// Wait a bit as the interface might have just been created.
			time.Sleep(2 * time.Second)
			c, i, newErr := ndp.Listen(iface, ndp.LinkLocal)
			if newErr != nil {
				log.Printf("Still waiting for the NDP connection: %v", newErr)
				continue
			}
			conn = c
			ip = i
			err = newErr
			break
		}
	}
	defer conn.Close()

	log.Printf("Listening on %s (%s)", config.Interface, ip)

	// Run the RA daemon.
	if err := runRADaemon(ctx, conn, ra, &config, iface); err != nil {
		log.Fatalf("RADV daemon error: %v", err)
	}

	log.Println("RADV daemon stopped")
}

func buildRouterAdvertisement(config *radvDaemonConfig, prefix netip.Prefix) *ndp.RouterAdvertisement {
	// Build prefix information option
	prefixInfo := &ndp.PrefixInformation{
		PrefixLength:                   uint8(prefix.Bits()),
		OnLink:                         config.PrefixOnLink,
		AutonomousAddressConfiguration: config.PrefixAutonomous,
		ValidLifetime:                  time.Duration(config.PrefixValidLifetime) * time.Second,
		PreferredLifetime:              time.Duration(config.PrefixPreferredLifetime) * time.Second,
		Prefix:                         prefix.Addr(),
	}

	options := []ndp.Option{prefixInfo}

	// Add DNS servers if specified
	if config.DNSServers != "" {
		servers := strings.Split(config.DNSServers, ",")
		var dnsAddrs []netip.Addr
		for _, server := range servers {
			server = strings.TrimSpace(server)
			if addr, err := netip.ParseAddr(server); err == nil {
				dnsAddrs = append(dnsAddrs, addr)
			} else {
				log.Printf("Warning: invalid DNS server address %s: %v", server, err)
			}
		}
		if len(dnsAddrs) > 0 {
			rdnss := &ndp.RecursiveDNSServer{
				Lifetime: time.Duration(config.RouterLifetime) * time.Second,
				Servers:  dnsAddrs,
			}
			options = append(options, rdnss)
		}
	}

	ra := &ndp.RouterAdvertisement{
		CurrentHopLimit:      uint8(config.HopLimit),
		ManagedConfiguration: config.ManagedConfig,
		OtherConfiguration:   config.OtherConfig,
		RouterLifetime:       time.Duration(config.RouterLifetime) * time.Second,
		ReachableTime:        0, // Unspecified
		RetransmitTimer:      0, // Unspecified
		Options:              options,
	}

	return ra
}

func runRADaemon(ctx context.Context, conn *ndp.Conn, ra *ndp.RouterAdvertisement, config *radvDaemonConfig, iface *net.Interface) error {
	// Set up periodic RA sending.
	ticker := time.NewTicker(time.Duration(config.MaxInterval) * time.Second)
	defer ticker.Stop()

	// Send initial RA.
	if err := sendRA(conn, ra, iface); err != nil {
		return fmt.Errorf("failed to send initial RA: %w", err)
	}

	// Listen for Router Solicitations in a separate goroutine.
	go func() {
		for {
			msg, _, _, err := conn.ReadFrom()
			if err != nil {
				select {
				case <-ctx.Done():
					return
				default:
					log.Printf("Error reading from NDP connection: %v", err)
					continue
				}
			}

			// Check if it's a Router Solicitation.
			if _, ok := msg.(*ndp.RouterSolicitation); ok {
				log.Println("Received Router Solicitation")
				// Send RA in response to Router Solicitation.
				if err := sendRA(conn, ra, iface); err != nil {
					log.Printf("Error sending RA in response to RS: %v", err)
				}
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			// Send periodic RA.
			if err := sendRA(conn, ra, iface); err != nil {
				log.Printf("Error sending periodic RA: %v", err)
			}
		}
	}
}

func sendRA(conn *ndp.Conn, ra *ndp.RouterAdvertisement, iface *net.Interface) error {
	// Send to all-nodes multicast address (ff02::1).
	dst := netip.MustParseAddr("ff02::1")

	if err := conn.WriteTo(ra, nil, dst); err != nil {
		return fmt.Errorf("failed to send RA: %w", err)
	}

	log.Println("Sent Router Advertisement")
	return nil
}
