//go:build linux && amd64
// +build linux,amd64

package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/andrei-zededa/monitor-system-usage/pkg/msuformat"
	"github.com/miekg/dns"
	probing "github.com/prometheus-community/pro-bing"
	"gopkg.in/yaml.v3"
)

const defaultDoHEndpoint = "https://dns.quad9.net/dns-query"

// imCfg is the YAML configuration for the internet-monitor mode.
type imCfg struct {
	OutputFile     string        `yaml:"output_file"`
	Interval       time.Duration `yaml:"interval"`
	PingCount      int           `yaml:"ping_count"`
	PingTimeout    time.Duration `yaml:"ping_timeout"`
	DNSTimeout     time.Duration `yaml:"dns_timeout"`
	HTTPTimeout    time.Duration `yaml:"http_timeout"`
	DoHEndpoint    string        `yaml:"doh_endpoint"`
	FlushEveryN    int           `yaml:"flush_every_n"`
	Destinations   []string      `yaml:"destinations"`
	PrivilegedICMP bool          `yaml:"privileged_icmp"`
}

func internetMonitorMain() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	cfgData, err := os.ReadFile(*imConfig)
	if err != nil {
		logger.Error("Failed to read config file", "path", *imConfig, "error", err)
		os.Exit(1)
	}

	var cfg imCfg
	if err := yaml.Unmarshal(cfgData, &cfg); err != nil {
		logger.Error("Failed to parse config file", "error", err)
		os.Exit(1)
	}

	if cfg.OutputFile == "" {
		logger.Error("output_file is required in config")
		os.Exit(1)
	}
	if len(cfg.Destinations) == 0 {
		logger.Error("destinations is empty in config")
		os.Exit(1)
	}
	if cfg.Interval <= 0 {
		cfg.Interval = 60 * time.Second
	}
	if cfg.PingCount <= 0 {
		cfg.PingCount = 5
	}
	if cfg.PingTimeout <= 0 {
		cfg.PingTimeout = 5 * time.Second
	}
	if cfg.DNSTimeout <= 0 {
		cfg.DNSTimeout = 5 * time.Second
	}
	if cfg.HTTPTimeout <= 0 {
		cfg.HTTPTimeout = 10 * time.Second
	}
	if cfg.DoHEndpoint == "" {
		cfg.DoHEndpoint = defaultDoHEndpoint
	}
	if cfg.FlushEveryN <= 0 {
		cfg.FlushEveryN = 1
	}

	w, err := msuformat.NewFileWriter(cfg.OutputFile)
	if err != nil {
		logger.Error("Failed to open MSU output file", "path", cfg.OutputFile, "error", err)
		os.Exit(1)
	}

	hostname, _ := os.Hostname()
	if err := w.WriteHeader(&msuformat.Header{
		TS:            msuformat.NowNanos(),
		MsuVer:        "internet-monitor-" + version,
		Hostname:      hostname,
		KernelOSType:  readKernelFile("ostype"),
		KernelRelease: readKernelFile("osrelease"),
		KernelVersion: readKernelFile("version"),
		IntervalNS:    cfg.Interval.Nanoseconds(),
		FlushEveryN:   cfg.FlushEveryN,
		CmdLine:       os.Args,
		EnvMode:       msuformat.EnvModeNone,
	}); err != nil {
		logger.Error("Failed to write MSU header", "error", err)
		os.Exit(1)
	}

	logger.Info("Internet monitor started",
		"output", cfg.OutputFile,
		"interval", cfg.Interval,
		"destinations", len(cfg.Destinations),
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

	dohClient := newHTTPSClient(cfg.HTTPTimeout, "")

	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()

	var seq int64
	runCycle(ctx, w, &cfg, seq, dohClient, logger)
	seq++
	if seq%int64(cfg.FlushEveryN) == 0 {
		if err := w.Flush(); err != nil {
			logger.Warn("MSU flush failed", "error", err)
		}
	}

	for {
		select {
		case <-ctx.Done():
			if err := w.Close(); err != nil {
				logger.Warn("MSU close failed", "error", err)
			}
			return
		case <-ticker.C:
			runCycle(ctx, w, &cfg, seq, dohClient, logger)
			seq++
			if seq%int64(cfg.FlushEveryN) == 0 {
				if err := w.Flush(); err != nil {
					logger.Warn("MSU flush failed", "error", err)
				}
			}
		}
	}
}

// runCycle probes every destination once.
func runCycle(ctx context.Context, w *msuformat.Writer, cfg *imCfg, seq int64, dohClient *http.Client, logger *slog.Logger) {
	for _, dest := range cfg.Destinations {
		probeDestination(ctx, w, cfg, seq, dest, dohClient, logger)
	}
}

// probeDestination runs both resolver rounds (system + DoH) against one URL.
func probeDestination(ctx context.Context, w *msuformat.Writer, cfg *imCfg, seq int64, destURL string, dohClient *http.Client, logger *slog.Logger) {
	u, err := url.Parse(destURL)
	if err != nil {
		writeSample(w, cfg, seq, "dns-system "+destURL, "", err.Error(), logger)
		writeSample(w, cfg, seq, "dns-doh "+destURL, "", err.Error(), logger)
		return
	}
	host := u.Hostname()
	if host == "" {
		errMsg := "URL has no hostname"
		writeSample(w, cfg, seq, "dns-system "+destURL, "", errMsg, logger)
		writeSample(w, cfg, seq, "dns-doh "+destURL, "", errMsg, logger)
		return
	}

	runRound(ctx, w, cfg, seq, "system", destURL, u, host, dohClient, logger)
	runRound(ctx, w, cfg, seq, "doh", destURL, u, host, dohClient, logger)
}

// runRound executes DNS → ICMP → HTTPS for a single resolver mode.
func runRound(ctx context.Context, w *msuformat.Writer, cfg *imCfg, seq int64, mode, destURL string, u *url.URL, host string, dohClient *http.Client, logger *slog.Logger) {
	dnsCtx, dnsCancel := context.WithTimeout(ctx, cfg.DNSTimeout)
	defer dnsCancel()

	var (
		ips      []net.IP
		resolver string
		dnsErr   error
		dnsRTT   time.Duration
	)
	switch mode {
	case "system":
		resolver = systemResolverName()
		ips, dnsRTT, dnsErr = resolveSystem(dnsCtx, host)
	case "doh":
		resolver = cfg.DoHEndpoint
		ips, dnsRTT, dnsErr = resolveDoH(dnsCtx, dohClient, cfg.DoHEndpoint, host)
	}

	dnsOut := formatDNSOut(resolver, ips, dnsRTT)
	dnsErrStr := ""
	if dnsErr != nil {
		dnsErrStr = dnsErr.Error()
	}
	writeSample(w, cfg, seq, "dns-"+mode+" "+destURL, dnsOut, dnsErrStr, logger)

	// ICMP and HTTPS still run even with no IPs — they'll record empty/skipped.
	icmpOut := runICMPRound(ctx, ips, cfg)
	writeSample(w, cfg, seq, "icmp-"+mode+" "+destURL, icmpOut, "", logger)

	httpsOut := runHTTPSRound(ctx, u, ips, cfg)
	writeSample(w, cfg, seq, "https-"+mode+" "+destURL, httpsOut, "", logger)
}

// writeSample is a tiny convenience wrapper that logs on failure rather than
// killing the daemon — a probe miss should not stop the loop.
func writeSample(w *msuformat.Writer, cfg *imCfg, seq int64, cmd, out, errStr string, logger *slog.Logger) {
	if err := w.WriteSample("B", cmd, "", seq, msuformat.NowNanos(), out, errStr); err != nil {
		logger.Warn("WriteSample failed", "cmd", cmd, "error", err)
	}
}

// --- DNS resolution ---------------------------------------------------------

func systemResolverName() string {
	// /etc/resolv.conf is the system source of truth used by net.DefaultResolver.
	data, err := os.ReadFile("/etc/resolv.conf")
	if err != nil {
		return "system"
	}
	var servers []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "nameserver ") {
			servers = append(servers, strings.TrimSpace(strings.TrimPrefix(line, "nameserver ")))
		}
	}
	if len(servers) == 0 {
		return "system"
	}
	return "system (" + strings.Join(servers, ",") + ")"
}

func resolveSystem(ctx context.Context, host string) ([]net.IP, time.Duration, error) {
	start := time.Now()
	addrs, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	rtt := time.Since(start)
	if err != nil {
		return nil, rtt, err
	}
	return uniqueSortedIPs(addrs), rtt, nil
}

func resolveDoH(ctx context.Context, client *http.Client, endpoint, host string) ([]net.IP, time.Duration, error) {
	start := time.Now()
	a4, errA := doHQuery(ctx, client, endpoint, host, dns.TypeA)
	a6, errAAAA := doHQuery(ctx, client, endpoint, host, dns.TypeAAAA)
	rtt := time.Since(start)

	all := append([]net.IP{}, a4...)
	all = append(all, a6...)

	var err error
	switch {
	case errA != nil && errAAAA != nil:
		err = fmt.Errorf("A: %v; AAAA: %v", errA, errAAAA)
	case errA != nil && len(a6) == 0:
		err = fmt.Errorf("A: %v", errA)
	case errAAAA != nil && len(a4) == 0:
		err = fmt.Errorf("AAAA: %v", errAAAA)
	}
	return uniqueSortedIPs(all), rtt, err
}

func doHQuery(ctx context.Context, client *http.Client, endpoint, host string, qtype uint16) ([]net.IP, error) {
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(host), qtype)
	m.RecursionDesired = true

	wire, err := m.Pack()
	if err != nil {
		return nil, fmt.Errorf("pack: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(wire))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/dns-message")
	req.Header.Set("Accept", "application/dns-message")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DoH HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	reply := new(dns.Msg)
	if err := reply.Unpack(body); err != nil {
		return nil, fmt.Errorf("unpack: %w", err)
	}

	var ips []net.IP
	for _, ans := range reply.Answer {
		switch r := ans.(type) {
		case *dns.A:
			ips = append(ips, r.A)
		case *dns.AAAA:
			ips = append(ips, r.AAAA)
		}
	}
	return ips, nil
}

func uniqueSortedIPs(in []net.IP) []net.IP {
	seen := make(map[string]struct{}, len(in))
	out := make([]net.IP, 0, len(in))
	for _, ip := range in {
		if ip == nil {
			continue
		}
		s := ip.String()
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, ip)
	}
	sort.Slice(out, func(i, j int) bool {
		iv4 := out[i].To4() != nil
		jv4 := out[j].To4() != nil
		if iv4 != jv4 {
			return iv4 // IPv4 first
		}
		return out[i].String() < out[j].String()
	})
	return out
}

func formatDNSOut(resolver string, ips []net.IP, rtt time.Duration) string {
	var b strings.Builder
	fmt.Fprintf(&b, "resolver: %s\n", resolver)
	for _, ip := range ips {
		if ip.To4() != nil {
			fmt.Fprintf(&b, "A: %s\n", ip)
		} else {
			fmt.Fprintf(&b, "AAAA: %s\n", ip)
		}
	}
	fmt.Fprintf(&b, "rtt_ms: %.2f\n", float64(rtt.Microseconds())/1000.0)
	return b.String()
}

// --- ICMP -------------------------------------------------------------------

func runICMPRound(ctx context.Context, ips []net.IP, cfg *imCfg) string {
	if len(ips) == 0 {
		return "no targets\n"
	}
	var (
		mu     sync.Mutex
		blocks = make([]string, 0, len(ips))
		wg     sync.WaitGroup
	)
	for _, ip := range ips {
		ip := ip
		wg.Add(1)
		go func() {
			defer wg.Done()
			block := pingOne(ctx, ip, cfg)
			mu.Lock()
			blocks = append(blocks, block)
			mu.Unlock()
		}()
	}
	wg.Wait()
	sort.Strings(blocks)
	return strings.Join(blocks, "---\n")
}

func pingOne(ctx context.Context, ip net.IP, cfg *imCfg) string {
	var b strings.Builder
	fmt.Fprintf(&b, "target: %s\n", ip)

	pinger, err := probing.NewPinger(ip.String())
	if err != nil {
		fmt.Fprintf(&b, "error: %v\n", err)
		return b.String()
	}
	pinger.Count = cfg.PingCount
	pinger.Timeout = cfg.PingTimeout
	pinger.SetPrivileged(cfg.PrivilegedICMP)

	doneCh := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			pinger.Stop()
		case <-doneCh:
		}
	}()

	runErr := pinger.Run()
	close(doneCh)

	stats := pinger.Statistics()
	fmt.Fprintf(&b, "sent=%d recv=%d loss=%.1f%%\n", stats.PacketsSent, stats.PacketsRecv, stats.PacketLoss)
	if stats.PacketsRecv > 0 {
		fmt.Fprintf(&b, "rtt min/avg/max: %.2f/%.2f/%.2f ms\n",
			float64(stats.MinRtt.Microseconds())/1000.0,
			float64(stats.AvgRtt.Microseconds())/1000.0,
			float64(stats.MaxRtt.Microseconds())/1000.0,
		)
	}
	if runErr != nil {
		fmt.Fprintf(&b, "error: %v\n", runErr)
	}
	return b.String()
}

// --- HTTPS ------------------------------------------------------------------

// newHTTPSClient builds an http.Client with given timeout. If serverName is
// non-empty, TLS SNI/verification uses that name regardless of the dialed
// host (used for "GET https://<ip>" with Host: <hostname>).
func newHTTPSClient(timeout time.Duration, serverName string) *http.Client {
	tr := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
	}
	if serverName != "" {
		tr.TLSClientConfig = &tls.Config{ServerName: serverName}
	}
	return &http.Client{Timeout: timeout, Transport: tr}
}

func runHTTPSRound(ctx context.Context, u *url.URL, ips []net.IP, cfg *imCfg) string {
	host := u.Hostname()
	var b strings.Builder

	// First: hit the original URL exactly as the user wrote it.
	{
		start := time.Now()
		status, err := doGET(ctx, newHTTPSClient(cfg.HTTPTimeout, ""), u.String(), "")
		dt := time.Since(start)
		writeHTTPSLine(&b, u.String(), "", status, err, dt)
	}

	// Then: hit each resolved IP with the original hostname as Host header.
	for _, ip := range ips {
		ipURL := *u
		if ip.To4() != nil {
			ipURL.Host = ip.String()
		} else {
			ipURL.Host = "[" + ip.String() + "]"
		}
		if u.Port() != "" {
			ipURL.Host += ":" + u.Port()
		}
		start := time.Now()
		status, err := doGET(ctx, newHTTPSClient(cfg.HTTPTimeout, host), ipURL.String(), host)
		dt := time.Since(start)
		writeHTTPSLine(&b, ipURL.String(), host, status, err, dt)
	}
	return b.String()
}

func doGET(ctx context.Context, client *http.Client, urlStr, hostHeader string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return "", err
	}
	if hostHeader != "" {
		req.Host = hostHeader
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.Status, nil
}

func writeHTTPSLine(b *strings.Builder, urlStr, hostHeader, status string, err error, dt time.Duration) {
	prefix := "GET " + urlStr
	if hostHeader != "" {
		prefix += " Host:" + hostHeader
	}
	if err != nil {
		fmt.Fprintf(b, "%s → ERROR %v (%.0fms)\n", prefix, err, float64(dt.Microseconds())/1000.0)
		return
	}
	fmt.Fprintf(b, "%s → %s (%.0fms)\n", prefix, status, float64(dt.Microseconds())/1000.0)
}

// --- misc -------------------------------------------------------------------

func readKernelFile(name string) string {
	data, err := os.ReadFile("/proc/sys/kernel/" + name)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
