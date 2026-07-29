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
	"golang.org/x/crypto/ssh/agent"

	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/exec"
)

// testServerOpts configures the in-process SSH server used by the tests.
type testServerOpts struct {
	password      string         // if set, accept this password
	authorizedKey gssh.PublicKey // if set, accept this public key
	allowTCPFwd   bool           // if set, enable direct-tcpip (local) forwarding
	// agentFwd makes the server act on agent-forwarding requests the way sshd
	// does — set up a per-session agent socket and export SSH_AUTH_SOCK into the
	// command's environment — and report what it saw on the session's stdout.
	// Off by default so the other tests' stdout stays exactly what they ran.
	agentFwd bool
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
			if opts.agentFwd {
				if sock, ok := serveForwardedAgent(t, s); ok {
					c.Env = append(os.Environ(), "SSH_AUTH_SOCK="+sock)
				}
			}
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

// testAgentKeyComment identifies the key held by the test agent, so a test can
// prove a listing came from THAT agent and not from the developer's own.
const testAgentKeyComment = "za-forward-agent-test-key"

// serveForwardedAgent mirrors what sshd does for an accepted agent-forwarding
// request: create a per-session unix socket, proxy it over the "auth-agent"
// channel back to the client, and hand the path to the command via
// SSH_AUTH_SOCK. It also dials its own socket and lists the keys, writing them to
// the session's stdout — that is the end-to-end proof that the channel really
// reaches the client's agent rather than just that a socket path exists.
func serveForwardedAgent(t *testing.T, s gssh.Session) (sock string, ok bool) {
	t.Helper()

	if !gssh.AgentRequested(s) {
		io.WriteString(s, "AGENT: none\n")
		return "", false
	}

	l, err := gssh.NewAgentListener()
	if err != nil {
		io.WriteString(s, "AGENT_LISTEN_ERR: "+err.Error()+"\n")
		return "", false
	}
	go gssh.ForwardAgentConnections(l, s)
	// The listener is torn down when the session ends, exactly like sshd's.
	go func() {
		<-s.Context().Done()
		_ = l.Close()
	}()

	conn, err := net.Dial("unix", l.Addr().String())
	if err != nil {
		io.WriteString(s, "AGENT_DIAL_ERR: "+err.Error()+"\n")
		return l.Addr().String(), true
	}
	defer conn.Close()

	keys, err := agent.NewClient(conn).List()
	if err != nil {
		io.WriteString(s, "AGENT_LIST_ERR: "+err.Error()+"\n")
		return l.Addr().String(), true
	}
	for _, k := range keys {
		io.WriteString(s, "AGENT_KEY: "+k.Comment+"\n")
	}

	return l.Addr().String(), true
}

// startTestAgent runs a real in-process SSH agent (a keyring holding one
// identifiable key) on a unix socket, and returns its path — the stand-in for the
// operator's $SSH_AUTH_SOCK.
func startTestAgent(t *testing.T) string {
	t.Helper()

	sock := filepath.Join(t.TempDir(), "agent.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen on agent socket: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate agent key: %v", err)
	}
	keyring := agent.NewKeyring()
	if err := keyring.Add(agent.AddedKey{PrivateKey: priv, Comment: testAgentKeyComment}); err != nil {
		t.Fatalf("add key to keyring: %v", err)
	}

	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() { _ = agent.ServeAgent(keyring, c) }()
		}
	}()

	return sock
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

// clientConfig builds a client config with key auth and a pinned host key.
func clientConfig(signer ssh.Signer, hostKey ssh.PublicKey) *ssh.ClientConfig {
	return &ssh.ClientConfig{
		User:            "tester",
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.FixedHostKey(hostKey),
		Timeout:         5 * time.Second,
	}
}

// TestSSHProxyJump verifies that the executor tunnels the target connection
// through a single jump host, exercising both command execution and SFTP (log
// writing) over the tunnel.
func TestSSHProxyJump(t *testing.T) {
	clientSigner := genSigner(t)
	// The jump host must allow direct-tcpip forwarding so the target connection
	// can be tunneled through it.
	jumpAddr, jumpKey := newTestServer(t, testServerOpts{
		authorizedKey: clientSigner.PublicKey(), allowTCPFwd: true,
	})
	targetAddr, targetKey := newTestServer(t, testServerOpts{
		authorizedKey: clientSigner.PublicKey(),
	})

	e := exec.NewSSH(exec.SSHParams{
		Addr:         targetAddr,
		ClientConfig: clientConfig(clientSigner, targetKey),
		Jumps: []exec.JumpHost{
			{Addr: jumpAddr, ClientConfig: clientConfig(clientSigner, jumpKey)},
		},
	})
	t.Cleanup(func() { _ = e.Close() })

	res, err := e.Run(context.Background(), t.TempDir(), "echo", "via jump")
	if err != nil {
		t.Fatalf("Run via jump: %v", err)
	}
	if strings.TrimSpace(res.Stdout) != "via jump" {
		t.Fatalf("stdout = %q, want %q", res.Stdout, "via jump")
	}
}

// TestSSHProxyJumpChain verifies tunneling through a chain of two jump hosts.
func TestSSHProxyJumpChain(t *testing.T) {
	clientSigner := genSigner(t)
	jump1Addr, jump1Key := newTestServer(t, testServerOpts{authorizedKey: clientSigner.PublicKey(), allowTCPFwd: true})
	jump2Addr, jump2Key := newTestServer(t, testServerOpts{authorizedKey: clientSigner.PublicKey(), allowTCPFwd: true})
	targetAddr, targetKey := newTestServer(t, testServerOpts{authorizedKey: clientSigner.PublicKey()})

	e := exec.NewSSH(exec.SSHParams{
		Addr:         targetAddr,
		ClientConfig: clientConfig(clientSigner, targetKey),
		Jumps: []exec.JumpHost{
			{Addr: jump1Addr, ClientConfig: clientConfig(clientSigner, jump1Key)},
			{Addr: jump2Addr, ClientConfig: clientConfig(clientSigner, jump2Key)},
		},
	})
	t.Cleanup(func() { _ = e.Close() })

	res, err := e.Run(context.Background(), t.TempDir(), "echo", "chain")
	if err != nil {
		t.Fatalf("Run via jump chain: %v", err)
	}
	if strings.TrimSpace(res.Stdout) != "chain" {
		t.Fatalf("stdout = %q, want %q", res.Stdout, "chain")
	}
}

// TestSSHProxyJumpBadJump verifies that a failure dialing the jump host surfaces
// as an error (and does not hang) rather than reaching the target.
func TestSSHProxyJumpBadJump(t *testing.T) {
	clientSigner := genSigner(t)
	targetAddr, targetKey := newTestServer(t, testServerOpts{authorizedKey: clientSigner.PublicKey()})

	// A closed listener address for the jump host: dialing it must fail.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	deadJump := ln.Addr().String()
	_ = ln.Close()

	e := exec.NewSSH(exec.SSHParams{
		Addr:         targetAddr,
		ClientConfig: clientConfig(clientSigner, targetKey),
		Jumps: []exec.JumpHost{
			{Addr: deadJump, ClientConfig: clientConfig(clientSigner, targetKey)},
		},
	})
	t.Cleanup(func() { _ = e.Close() })

	if _, err := e.Run(context.Background(), t.TempDir(), "true"); err == nil {
		t.Fatal("expected an error when the jump host is unreachable")
	}
}

// TestSSHForwardAgent checks the whole agent-forwarding path: the executor
// registers the auth-agent channel handler on the client, requests forwarding for
// the session, and the command on the target ends up with an SSH_AUTH_SOCK that
// reaches the operator's real agent. This is what lets a target-side `ssh` (as in
// a zedamigo_wait_until probe) authenticate with keys that were never copied to
// the target.
func TestSSHForwardAgent(t *testing.T) {
	agentSock := startTestAgent(t)

	clientSigner := genSigner(t)
	addr, hostKey := newTestServer(t, testServerOpts{
		authorizedKey: clientSigner.PublicKey(),
		agentFwd:      true,
	})

	e := exec.NewSSH(exec.SSHParams{
		Addr:         addr,
		ClientConfig: clientConfig(clientSigner, hostKey),
		AgentSocket:  agentSock,
	})
	t.Cleanup(func() { _ = e.Close() })

	res, err := e.Run(context.Background(), t.TempDir(), "sh", "-c", `printf 'SOCK=%s\n' "$SSH_AUTH_SOCK"`)
	if err != nil {
		t.Fatalf("Run: %v (stderr: %s)", err, res.Stderr)
	}

	// The forwarded channel reached the keyring started by this test.
	if !strings.Contains(res.Stdout, "AGENT_KEY: "+testAgentKeyComment) {
		t.Fatalf("the forwarded agent did not serve the test key; stdout:\n%s", res.Stdout)
	}
	// ... and the command itself got a usable SSH_AUTH_SOCK.
	if strings.Contains(res.Stdout, "SOCK=\n") || !strings.Contains(res.Stdout, "SOCK=/") {
		t.Fatalf("SSH_AUTH_SOCK was not exported into the command; stdout:\n%s", res.Stdout)
	}
}

// TestSSHForwardAgentOffByDefault is the guard on the default: with no
// AgentSocket the executor must not request forwarding at all, so no agent socket
// exists on the target and none of the operator's keys are reachable from it.
func TestSSHForwardAgentOffByDefault(t *testing.T) {
	clientSigner := genSigner(t)
	addr, hostKey := newTestServer(t, testServerOpts{
		authorizedKey: clientSigner.PublicKey(),
		agentFwd:      true,
	})

	// Note: no AgentSocket.
	e := exec.NewSSH(exec.SSHParams{
		Addr:         addr,
		ClientConfig: clientConfig(clientSigner, hostKey),
	})
	t.Cleanup(func() { _ = e.Close() })

	res, err := e.Run(context.Background(), t.TempDir(), "sh", "-c", `printf 'SOCK=%s\n' "$SSH_AUTH_SOCK"`)
	if err != nil {
		t.Fatalf("Run: %v (stderr: %s)", err, res.Stderr)
	}

	if !strings.Contains(res.Stdout, "AGENT: none") {
		t.Fatalf("expected the server to see no forwarding request; stdout:\n%s", res.Stdout)
	}
	if !strings.Contains(res.Stdout, "SOCK=\n") {
		t.Fatalf("expected an empty SSH_AUTH_SOCK on the target; stdout:\n%s", res.Stdout)
	}
	if strings.Contains(res.Stdout, "AGENT_KEY:") {
		t.Fatalf("no key must be reachable from the target by default; stdout:\n%s", res.Stdout)
	}
}

// TestSSHForwardAgentBadSocket checks that a broken SSH_AUTH_SOCK is reported
// when the connection is established, rather than silently later.
func TestSSHForwardAgentBadSocket(t *testing.T) {
	clientSigner := genSigner(t)
	addr, hostKey := newTestServer(t, testServerOpts{
		authorizedKey: clientSigner.PublicKey(),
		agentFwd:      true,
	})

	e := exec.NewSSH(exec.SSHParams{
		Addr:         addr,
		ClientConfig: clientConfig(clientSigner, hostKey),
		AgentSocket:  filepath.Join(t.TempDir(), "no-such-agent.sock"),
	})
	t.Cleanup(func() { _ = e.Close() })

	_, err := e.Run(context.Background(), t.TempDir(), "true")
	if err == nil {
		t.Fatal("expected an error for an unusable agent socket")
	}
	if !strings.Contains(err.Error(), "can't forward the SSH agent") {
		t.Fatalf("error should name agent forwarding as the cause, got: %v", err)
	}
}

// TestSSHForwardAgentOncePerConnection is the regression test for requesting
// agent forwarding per session instead of per connection. Against a real sshd that
// made the FIRST command work and every command after it fail with "forwarding
// request denied", because sshd honors auth-agent-req only once per connection.
// It is checked against sshdLike rather than the gliderlabs server, which replies
// true to every request and so cannot show the difference.
func TestSSHForwardAgentOncePerConnection(t *testing.T) {
	agentSock := startTestAgent(t)
	clientSigner := genSigner(t)
	srv := newSSHDLike(t, clientSigner.PublicKey(), sshdLikeOpts{})

	e := exec.NewSSH(exec.SSHParams{
		Addr:         srv.Addr,
		ClientConfig: clientConfig(clientSigner, srv.HostKey),
		AgentSocket:  agentSock,
	})
	t.Cleanup(func() { _ = e.Close() })

	ctx := context.Background()
	logDir := t.TempDir()

	// Several commands over the one connection: each must succeed AND see the
	// forwarded agent. Before the fix, only the first one did.
	var lastSock string
	for i := 1; i <= 3; i++ {
		res, err := e.Run(ctx, logDir, "sh", "-c", `printf 'SOCK=%s' "$SSH_AUTH_SOCK"`)
		if err != nil {
			t.Fatalf("command %d failed: %v (stderr: %s)", i, err, res.Stderr)
		}
		sock := strings.TrimPrefix(strings.TrimSpace(res.Stdout), "SOCK=")
		if sock == "" {
			t.Fatalf("command %d got no forwarded SSH_AUTH_SOCK (stdout: %q)", i, res.Stdout)
		}
		if lastSock != "" && sock != lastSock {
			t.Fatalf("command %d saw a different agent socket (%q, was %q): "+
				"forwarding should be set up once per connection", i, sock, lastSock)
		}
		lastSock = sock
	}

	if got := srv.AgentReqs.Load(); got != 1 {
		t.Fatalf("auth-agent-req was sent %d times; a real sshd honors exactly 1 per connection", got)
	}

	// The socket still reaches the operator's agent, from outside the session that
	// requested forwarding and after it closed — that is the connection-scoped
	// property the fix relies on. The "target" is this machine, so the test can
	// dial the path the remote command reported.
	conn, err := net.Dial("unix", lastSock)
	if err != nil {
		t.Fatalf("dial the forwarded agent socket %q: %v", lastSock, err)
	}
	defer conn.Close()

	keys, err := agent.NewClient(conn).List()
	if err != nil {
		t.Fatalf("list keys over the forwarded agent: %v", err)
	}
	var comments []string
	for _, k := range keys {
		comments = append(comments, k.Comment)
	}
	if len(keys) != 1 || comments[0] != testAgentKeyComment {
		t.Fatalf("forwarded agent served %v, want just %q", comments, testAgentKeyComment)
	}
}

// TestSSHForwardAgentRefusedAtConnect checks the AllowAgentForwarding=no case: it
// must fail once, when connecting, with an error that names the cause — not
// silently leave every later command without an agent.
func TestSSHForwardAgentRefusedAtConnect(t *testing.T) {
	agentSock := startTestAgent(t)
	clientSigner := genSigner(t)
	srv := newSSHDLike(t, clientSigner.PublicKey(), sshdLikeOpts{denyAgent: true})

	e := exec.NewSSH(exec.SSHParams{
		Addr:         srv.Addr,
		ClientConfig: clientConfig(clientSigner, srv.HostKey),
		AgentSocket:  agentSock,
	})
	t.Cleanup(func() { _ = e.Close() })

	_, err := e.Run(context.Background(), t.TempDir(), "true")
	if err == nil {
		t.Fatal("expected an error when the target refuses agent forwarding")
	}
	if !strings.Contains(err.Error(), "refused SSH agent forwarding") ||
		!strings.Contains(err.Error(), "AllowAgentForwarding") {
		t.Fatalf("the error should name the cause and the fix, got: %v", err)
	}
}

// TestSSHNoAgentRequestWhenDisabled guards the default on the faithful server too:
// with no AgentSocket the executor must not send auth-agent-req at all.
func TestSSHNoAgentRequestWhenDisabled(t *testing.T) {
	clientSigner := genSigner(t)
	srv := newSSHDLike(t, clientSigner.PublicKey(), sshdLikeOpts{})

	e := exec.NewSSH(exec.SSHParams{
		Addr:         srv.Addr,
		ClientConfig: clientConfig(clientSigner, srv.HostKey),
	})
	t.Cleanup(func() { _ = e.Close() })

	res, err := e.Run(context.Background(), t.TempDir(), "sh", "-c", `printf 'SOCK=%s' "$SSH_AUTH_SOCK"`)
	if err != nil {
		t.Fatalf("Run: %v (stderr: %s)", err, res.Stderr)
	}
	if got := strings.TrimSpace(res.Stdout); got != "SOCK=" {
		t.Fatalf("expected no agent socket on the target, got %q", got)
	}
	if got := srv.AgentReqs.Load(); got != 0 {
		t.Fatalf("auth-agent-req was sent %d times with forwarding disabled, want 0", got)
	}
}
