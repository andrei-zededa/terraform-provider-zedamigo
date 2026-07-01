package exec_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"io"
	"net"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	gssh "github.com/gliderlabs/ssh"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/exec"
)

// testServerOpts configures the in-process SSH server used by the tests.
type testServerOpts struct {
	password      string         // if set, accept this password
	authorizedKey gssh.PublicKey // if set, accept this public key
	allowTCPFwd   bool           // if set, enable direct-tcpip (local) forwarding
}

// newTestServer starts an in-process SSH server on 127.0.0.1 that executes
// commands via /bin/sh -c (mirroring real sshd) and serves the SFTP subsystem
// with github.com/pkg/sftp. It returns the listen address and the host key.
func newTestServer(t *testing.T, opts testServerOpts) (addr string, hostKey ssh.PublicKey) {
	t.Helper()

	hostSigner := genSigner(t)

	srv := &gssh.Server{
		Handler: func(s gssh.Session) {
			c := osexec.Command("/bin/sh", "-c", s.RawCommand())
			c.Stdout = s
			c.Stderr = s.Stderr()
			if err := c.Run(); err != nil {
				var ee *osexec.ExitError
				if errors.As(err, &ee) {
					_ = s.Exit(ee.ExitCode())
					return
				}
				io.WriteString(s.Stderr(), err.Error())
				_ = s.Exit(127)
				return
			}
			_ = s.Exit(0)
		},
		SubsystemHandlers: map[string]gssh.SubsystemHandler{
			"sftp": func(s gssh.Session) {
				server, err := sftp.NewServer(s)
				if err != nil {
					return
				}
				_ = server.Serve()
			},
		},
	}
	srv.AddHostKey(hostSigner)

	if opts.allowTCPFwd {
		srv.LocalPortForwardingCallback = func(_ gssh.Context, _ string, _ uint32) bool { return true }
		srv.ChannelHandlers = map[string]gssh.ChannelHandler{
			"session":      gssh.DefaultSessionHandler,
			"direct-tcpip": gssh.DirectTCPIPHandler,
		}
	}

	if opts.password != "" {
		srv.PasswordHandler = func(_ gssh.Context, password string) bool {
			return password == opts.password
		}
	}
	if opts.authorizedKey != nil {
		srv.PublicKeyHandler = func(_ gssh.Context, key gssh.PublicKey) bool {
			return gssh.KeysEqual(key, opts.authorizedKey)
		}
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	return ln.Addr().String(), hostSigner.PublicKey()
}

func genSigner(t *testing.T) ssh.Signer {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("signer: %v", err)
	}
	return signer
}

// newExecutor builds a SSHExecutor connected to addr using key auth and a
// pinned host key.
func newExecutor(t *testing.T, addr string, hostKey ssh.PublicKey, clientSigner ssh.Signer) *exec.SSHExecutor {
	t.Helper()
	cfg := &ssh.ClientConfig{
		User:            "tester",
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(clientSigner)},
		HostKeyCallback: ssh.FixedHostKey(hostKey),
		Timeout:         5 * time.Second,
	}
	e := exec.NewSSH(exec.SSHParams{Addr: addr, ClientConfig: cfg})
	t.Cleanup(func() { _ = e.Close() })
	return e
}

func TestSSHRun(t *testing.T) {
	clientSigner := genSigner(t)
	addr, hostKey := newTestServer(t, testServerOpts{authorizedKey: clientSigner.PublicKey()})
	e := newExecutor(t, addr, hostKey, clientSigner)

	ctx := context.Background()
	logDir := t.TempDir()

	res, err := e.Run(ctx, logDir, "echo", "hello world")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", res.ExitCode)
	}
	if strings.TrimSpace(res.Stdout) != "hello world" {
		t.Fatalf("stdout = %q, want %q", res.Stdout, "hello world")
	}
	// The per-command log files must have been written on the (loopback) target.
	if res.Logs.Stdout == "" {
		t.Fatal("expected a stdout log path")
	}
	if b, err := os.ReadFile(res.Logs.Stdout); err != nil {
		t.Fatalf("read stdout log: %v", err)
	} else if strings.TrimSpace(string(b)) != "hello world" {
		t.Fatalf("stdout log = %q", string(b))
	}

	// Non-zero exit code is captured.
	res, err = e.Run(ctx, logDir, "sh", "-c", "exit 7")
	if err == nil {
		t.Fatal("expected error for non-zero exit")
	}
	if res.ExitCode != 7 {
		t.Fatalf("exit code = %d, want 7", res.ExitCode)
	}
}

func TestSSHFileOps(t *testing.T) {
	clientSigner := genSigner(t)
	addr, hostKey := newTestServer(t, testServerOpts{authorizedKey: clientSigner.PublicKey()})
	e := newExecutor(t, addr, hostKey, clientSigner)

	ctx := context.Background()
	base := t.TempDir()
	dir := filepath.Join(base, "a", "b")

	if err := e.MkdirAll(ctx, dir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	f := filepath.Join(dir, "file.txt")
	if err := e.WriteFile(ctx, f, []byte("data"), 0o640); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if b, err := e.ReadFile(ctx, f); err != nil || string(b) != "data" {
		t.Fatalf("ReadFile = %q, %v", string(b), err)
	}
	if fi, err := e.Stat(ctx, f); err != nil {
		t.Fatalf("Stat: %v", err)
	} else if fi.Size() != 4 {
		t.Fatalf("size = %d, want 4", fi.Size())
	}

	// Stat of a missing file must satisfy exec.IsNotExist.
	if _, err := e.Stat(ctx, filepath.Join(dir, "nope")); !exec.IsNotExist(err) {
		t.Fatalf("Stat missing: expected IsNotExist, got %v", err)
	}

	// OpenWrite + OpenRead.
	f2 := filepath.Join(dir, "stream.txt")
	w, err := e.OpenWrite(ctx, f2, 0o644)
	if err != nil {
		t.Fatalf("OpenWrite: %v", err)
	}
	if _, err := io.WriteString(w, "streamed"); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	rc, err := e.OpenRead(ctx, f2)
	if err != nil {
		t.Fatalf("OpenRead: %v", err)
	}
	got, _ := io.ReadAll(rc)
	rc.Close()
	if string(got) != "streamed" {
		t.Fatalf("OpenRead = %q", string(got))
	}

	// Rename (overwrite semantics) + ReadDir.
	f3 := filepath.Join(dir, "renamed.txt")
	if err := e.Rename(ctx, f2, f3); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	entries, err := e.ReadDir(ctx, dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 2 { // file.txt + renamed.txt
		t.Fatalf("ReadDir len = %d, want 2", len(entries))
	}

	// CopyFile + Upload.
	if _, err := e.CopyFile(ctx, f, filepath.Join(dir, "copy.txt")); err != nil {
		t.Fatalf("CopyFile: %v", err)
	}
	localSrc := filepath.Join(base, "local.txt")
	if err := os.WriteFile(localSrc, []byte("uploaded"), 0o644); err != nil {
		t.Fatalf("write local: %v", err)
	}
	if _, err := e.Upload(ctx, localSrc, filepath.Join(dir, "up.txt"), 0o600); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if b, err := e.ReadFile(ctx, filepath.Join(dir, "up.txt")); err != nil || string(b) != "uploaded" {
		t.Fatalf("uploaded content = %q, %v", string(b), err)
	}

	// Remove (recursive).
	if err := e.Remove(ctx, filepath.Join(base, "a")); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := e.Stat(ctx, dir); !exec.IsNotExist(err) {
		t.Fatalf("after Remove: expected IsNotExist, got %v", err)
	}
}

// TestSSHRunDetachedAndProcess verifies that a detached process survives the
// SSH session that launched it, and that IsRunning/Kill work over SSH.
func TestSSHRunDetachedAndProcess(t *testing.T) {
	clientSigner := genSigner(t)
	addr, hostKey := newTestServer(t, testServerOpts{authorizedKey: clientSigner.PublicKey()})
	e := newExecutor(t, addr, hostKey, clientSigner)

	ctx := context.Background()
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "pid")

	// The detached command records its own PID (which survives the exec into
	// sleep) and then sleeps, so we can control it deterministically.
	_, err := e.RunDetached(ctx, dir, "sh", "-c",
		"echo $$ > "+pidFile+"; exec sleep 30")
	if err != nil {
		t.Fatalf("RunDetached: %v", err)
	}

	// The launching session has returned; the process must still be running.
	var pid int
	deadline := time.Now().Add(5 * time.Second)
	for {
		b, rerr := e.ReadFile(ctx, pidFile)
		if rerr == nil {
			if p, perr := strconv.Atoi(strings.TrimSpace(string(b))); perr == nil && p > 0 {
				pid = p
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("pid file never appeared: %v", rerr)
		}
		time.Sleep(100 * time.Millisecond)
	}

	running, err := e.IsRunning(ctx, pid, "")
	if err != nil {
		t.Fatalf("IsRunning: %v", err)
	}
	if !running {
		t.Fatalf("detached process %d should be running", pid)
	}

	if err := e.Kill(ctx, pid, syscall.SIGKILL); err != nil {
		t.Fatalf("Kill: %v", err)
	}

	// Wait for it to die.
	deadline = time.Now().Add(5 * time.Second)
	for {
		running, err = e.IsRunning(ctx, pid, "")
		if err != nil {
			t.Fatalf("IsRunning after kill: %v", err)
		}
		if !running {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("process %d still running after kill", pid)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// TestSSHDial verifies tunneled dialing through SSH (the Dial method's
// goroutine + timeout plumbing). It uses direct-tcpip forwarding because the
// in-process test server does not implement unix streamlocal channels; the
// executor's Dial logic is identical for "tcp" and "unix" (both delegate to
// (*ssh.Client).Dial), and the unix path is exercised end-to-end by the SSH
// e2e against a real sshd.
func TestSSHDial(t *testing.T) {
	clientSigner := genSigner(t)
	addr, hostKey := newTestServer(t, testServerOpts{authorizedKey: clientSigner.PublicKey(), allowTCPFwd: true})
	e := newExecutor(t, addr, hostKey, clientSigner)

	// An echo server on a TCP socket "on the target" (loopback).
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen tcp: %v", err)
	}
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		io.Copy(c, c)
	}()

	conn, err := e.Dial(context.Background(), "tcp", ln.Addr().String(), 5*time.Second)
	if err != nil {
		t.Fatalf("Dial tcp: %v", err)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte("ping\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	buf := make([]byte, 5)
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buf) != "ping\n" {
		t.Fatalf("echo = %q, want %q", string(buf), "ping\n")
	}
}

func TestSSHLookPath(t *testing.T) {
	clientSigner := genSigner(t)
	addr, hostKey := newTestServer(t, testServerOpts{authorizedKey: clientSigner.PublicKey()})
	e := newExecutor(t, addr, hostKey, clientSigner)

	ctx := context.Background()
	p, err := e.LookPath(ctx, "sh")
	if err != nil {
		t.Fatalf("LookPath sh: %v", err)
	}
	if !strings.HasSuffix(p, "/sh") {
		t.Fatalf("LookPath sh = %q", p)
	}
	if _, err := e.LookPath(ctx, "this-binary-does-not-exist-zzz"); err == nil {
		t.Fatal("expected LookPath error for missing binary")
	}
}

func TestSSHAuthPassword(t *testing.T) {
	addr, hostKey := newTestServer(t, testServerOpts{password: "s3cr3t"})
	cfg := &ssh.ClientConfig{
		User:            "tester",
		Auth:            []ssh.AuthMethod{ssh.Password("s3cr3t")},
		HostKeyCallback: ssh.FixedHostKey(hostKey),
		Timeout:         5 * time.Second,
	}
	e := exec.NewSSH(exec.SSHParams{Addr: addr, ClientConfig: cfg})
	defer e.Close()

	if _, err := e.Run(context.Background(), t.TempDir(), "true"); err != nil {
		t.Fatalf("password auth Run: %v", err)
	}
}

func TestSSHHostKeyMismatch(t *testing.T) {
	clientSigner := genSigner(t)
	addr, _ := newTestServer(t, testServerOpts{authorizedKey: clientSigner.PublicKey()})

	// Pin a DIFFERENT host key — the connection must be rejected.
	wrong := genSigner(t).PublicKey()
	cfg := &ssh.ClientConfig{
		User:            "tester",
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(clientSigner)},
		HostKeyCallback: ssh.FixedHostKey(wrong),
		Timeout:         5 * time.Second,
	}
	e := exec.NewSSH(exec.SSHParams{Addr: addr, ClientConfig: cfg})
	defer e.Close()

	if _, err := e.Run(context.Background(), t.TempDir(), "true"); err == nil {
		t.Fatal("expected host key mismatch to fail the connection")
	}
}
