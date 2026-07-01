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
		"ZEDAMIGO_SSH_INSECURE",
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
