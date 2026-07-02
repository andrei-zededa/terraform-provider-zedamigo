// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"golang.org/x/crypto/ssh"
)

// clearSSHEnv isolates the test from any ZEDAMIGO_SSH_* environment fallbacks.
func clearSSHEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"ZEDAMIGO_SSH_USER", "ZEDAMIGO_SSH_PASSWORD", "ZEDAMIGO_SSH_PRIVATE_KEY",
		"ZEDAMIGO_SSH_PRIVATE_KEY_FILE", "ZEDAMIGO_SSH_PRIVATE_KEY_PASSPHRASE",
		"ZEDAMIGO_SSH_USE_AGENT", "ZEDAMIGO_SSH_KNOWN_HOSTS", "ZEDAMIGO_SSH_HOST_KEY",
		"ZEDAMIGO_SSH_INSECURE", "ZEDAMIGO_SSH_PROXY_JUMP",
	} {
		t.Setenv(k, "")
	}
}

func writeTestKey(t *testing.T) string {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("genkey: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	f := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(f, pemBytes, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	return f
}

func TestBuildSSHClientConfig_NoAuth(t *testing.T) {
	clearSSHEnv(t)
	if _, err := buildSSHClientConfig(&SSHModel{}); err == nil {
		t.Fatal("expected an error when no authentication method is configured")
	}
}

func TestBuildSSHClientConfig_Password(t *testing.T) {
	clearSSHEnv(t)
	cfg, err := buildSSHClientConfig(&SSHModel{
		User:                  types.StringValue("bob"),
		Password:              types.StringValue("hunter2"),
		InsecureIgnoreHostKey: types.BoolValue(true),
	})
	if err != nil {
		t.Fatalf("buildSSHClientConfig: %v", err)
	}
	if cfg.User != "bob" {
		t.Fatalf("user = %q, want bob", cfg.User)
	}
	if len(cfg.Auth) != 1 {
		t.Fatalf("auth methods = %d, want 1", len(cfg.Auth))
	}
	if cfg.HostKeyCallback == nil {
		t.Fatal("expected a host key callback")
	}
}

func TestBuildSSHClientConfig_PrivateKeyFile(t *testing.T) {
	clearSSHEnv(t)
	keyFile := writeTestKey(t)
	cfg, err := buildSSHClientConfig(&SSHModel{
		PrivateKeyFile:        types.StringValue(keyFile),
		InsecureIgnoreHostKey: types.BoolValue(true),
	})
	if err != nil {
		t.Fatalf("buildSSHClientConfig: %v", err)
	}
	if len(cfg.Auth) != 1 {
		t.Fatalf("auth methods = %d, want 1", len(cfg.Auth))
	}
}

func TestBuildSSHClientConfig_BadKey(t *testing.T) {
	clearSSHEnv(t)
	f := filepath.Join(t.TempDir(), "bad")
	if err := os.WriteFile(f, []byte("not a key"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := buildSSHClientConfig(&SSHModel{
		PrivateKeyFile:        types.StringValue(f),
		InsecureIgnoreHostKey: types.BoolValue(true),
	}); err == nil {
		t.Fatal("expected an error for an invalid private key")
	}
}

func TestBuildHostKeyCallback(t *testing.T) {
	clearSSHEnv(t)

	// insecure -> a (non-verifying) callback, no error.
	if cb, err := buildHostKeyCallback(&SSHModel{InsecureIgnoreHostKey: types.BoolValue(true)}); err != nil || cb == nil {
		t.Fatalf("insecure: cb=%v err=%v", cb, err)
	}

	// a valid pinned host_key -> FixedHostKey callback.
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	signer, _ := ssh.NewSignerFromKey(priv)
	authLine := string(ssh.MarshalAuthorizedKey(signer.PublicKey()))
	if cb, err := buildHostKeyCallback(&SSHModel{HostKey: types.StringValue(authLine)}); err != nil || cb == nil {
		t.Fatalf("host_key: cb=%v err=%v", cb, err)
	}

	// a malformed host_key -> error.
	if _, err := buildHostKeyCallback(&SSHModel{HostKey: types.StringValue("not-a-key")}); err == nil {
		t.Fatal("expected an error for a malformed host_key")
	}
}

func TestBuildSSHJumpChain(t *testing.T) {
	clearSSHEnv(t)
	// Make the ~/.ssh/known_hosts fallback deterministic (absent) so the
	// fail-closed case below does not accidentally find a real file.
	t.Setenv("HOME", t.TempDir())

	target := &ssh.ClientConfig{
		User:    "alice",
		Auth:    []ssh.AuthMethod{ssh.Password("x")},
		Timeout: 15 * time.Second,
	}

	// No proxy_jump -> no hops, no error.
	if hops, err := buildSSHJumpChain(&SSHModel{}, target); err != nil || hops != nil {
		t.Fatalf("empty proxy_jump: hops=%v err=%v", hops, err)
	}

	// A single jump host with an explicit user + port, insecure host keys.
	hops, err := buildSSHJumpChain(&SSHModel{
		ProxyJump:             types.StringValue("root@localhost:11022"),
		InsecureIgnoreHostKey: types.BoolValue(true),
	}, target)
	if err != nil {
		t.Fatalf("single hop: %v", err)
	}
	if len(hops) != 1 {
		t.Fatalf("hops = %d, want 1", len(hops))
	}
	if hops[0].Addr != "localhost:11022" {
		t.Fatalf("addr = %q, want localhost:11022", hops[0].Addr)
	}
	if hops[0].ClientConfig.User != "root" {
		t.Fatalf("user = %q, want root", hops[0].ClientConfig.User)
	}
	// Auth methods and timeout are inherited from the target config.
	if len(hops[0].ClientConfig.Auth) != len(target.Auth) {
		t.Fatalf("auth methods = %d, want %d", len(hops[0].ClientConfig.Auth), len(target.Auth))
	}
	if hops[0].ClientConfig.Timeout != target.Timeout {
		t.Fatalf("timeout = %v, want %v", hops[0].ClientConfig.Timeout, target.Timeout)
	}
	if hops[0].ClientConfig.HostKeyCallback == nil {
		t.Fatal("expected a host key callback")
	}

	// A chain of two hosts; the second omits the user (inherits the target's)
	// and the port (defaults to 22). Whitespace around entries is tolerated.
	hops, err = buildSSHJumpChain(&SSHModel{
		ProxyJump:             types.StringValue("bob@bastion1:2222, bastion2"),
		InsecureIgnoreHostKey: types.BoolValue(true),
	}, target)
	if err != nil {
		t.Fatalf("chain: %v", err)
	}
	if len(hops) != 2 {
		t.Fatalf("hops = %d, want 2", len(hops))
	}
	if hops[0].Addr != "bastion1:2222" || hops[0].ClientConfig.User != "bob" {
		t.Fatalf("hop0 = %q/%q, want bastion1:2222/bob", hops[0].Addr, hops[0].ClientConfig.User)
	}
	if hops[1].Addr != "bastion2:22" {
		t.Fatalf("hop1 addr = %q, want bastion2:22", hops[1].Addr)
	}
	if hops[1].ClientConfig.User != "alice" {
		t.Fatalf("hop1 user = %q, want alice (inherited)", hops[1].ClientConfig.User)
	}

	// The proxy_jump value is also read from the environment fallback.
	t.Setenv("ZEDAMIGO_SSH_PROXY_JUMP", "envhost:2022")
	hops, err = buildSSHJumpChain(&SSHModel{
		InsecureIgnoreHostKey: types.BoolValue(true),
	}, target)
	if err != nil || len(hops) != 1 || hops[0].Addr != "envhost:2022" {
		t.Fatalf("env fallback: hops=%v err=%v", hops, err)
	}
	t.Setenv("ZEDAMIGO_SSH_PROXY_JUMP", "")

	// proxy_jump with no way to verify the jump host's key must fail closed
	// (host_key pins only the final target, so it does not count here).
	if _, err := buildSSHJumpChain(&SSHModel{
		ProxyJump: types.StringValue("bastion"),
		HostKey:   types.StringValue("ssh-ed25519 AAAA..."),
	}, target); err == nil {
		t.Fatal("expected an error: proxy_jump with no jump host key verification")
	}
}

func TestParseJumpSpec(t *testing.T) {
	cases := []struct {
		in               string
		user, host, port string
	}{
		{"host", "", "host", "22"},
		{"host:2222", "", "host", "2222"},
		{"root@host", "root", "host", "22"},
		{"root@host:11022", "root", "host", "11022"},
		{"[::1]:22", "", "::1", "22"},
		{"user@[2001:db8::1]:2222", "user", "2001:db8::1", "2222"},
		{"[2001:db8::1]", "", "2001:db8::1", "22"},
	}
	for _, c := range cases {
		user, host, port := parseJumpSpec(c.in)
		if user != c.user || host != c.host || port != c.port {
			t.Errorf("parseJumpSpec(%q) = %q/%q/%q, want %q/%q/%q",
				c.in, user, host, port, c.user, c.host, c.port)
		}
	}
}
