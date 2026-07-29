// SPDX-License-Identifier: MPL-2.0

//go:build linux

package provider

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"io"
	"net"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gssh "github.com/gliderlabs/ssh"
	"github.com/matryer/is"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/exec"
)

// The tests below drive the very same wait loop through a real SSHExecutor
// instead of a LocalExecutor. That is the whole point of the resource — the
// identical configuration must work for a remote target — and the remote path is
// not merely a re-run of the local one: the script is written over SFTP, and a
// non-zero exit arrives as an *ssh.ExitError rather than an *os/exec.ExitError.
// If classifyProbe got that wrong, every "not ready yet" answer from a remote
// target would be misread as "the probe could not be run" and the barrier would
// abort on the first attempt instead of retrying.
//
// The server is a minimal in-process one, mirroring internal/exec/ssh_test.go
// (which cannot be imported: it lives in package exec_test).

// newWaitSSHServer starts an in-process SSH server on 127.0.0.1 that runs
// commands via /bin/sh -c the way real sshd does, and serves the SFTP subsystem.
func newWaitSSHServer(t *testing.T, authorizedKey gssh.PublicKey) (addr string, hostKey ssh.PublicKey) {
	t.Helper()

	hostSigner := newWaitSSHSigner(t)

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
				_, _ = io.WriteString(s.Stderr(), err.Error())
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
		PublicKeyHandler: func(_ gssh.Context, key gssh.PublicKey) bool {
			return gssh.KeysEqual(key, authorizedKey)
		},
	}
	srv.AddHostKey(hostSigner)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	return ln.Addr().String(), hostSigner.PublicKey()
}

func newWaitSSHSigner(t *testing.T) ssh.Signer {
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

// newWaitSSHHarness returns a waitHarness backed by an SSHExecutor, with the
// probe script written onto the "target" exactly the way Create does it: over the
// executor, then chmod'ed.
func newWaitSSHHarness(t *testing.T, body string) *waitHarness {
	t.Helper()
	is := is.New(t)
	ctx := context.Background()

	clientSigner := newWaitSSHSigner(t)
	addr, hostKey := newWaitSSHServer(t, clientSigner.PublicKey())

	ex := exec.NewSSH(exec.SSHParams{
		Addr: addr,
		ClientConfig: &ssh.ClientConfig{
			User:            "tester",
			Auth:            []ssh.AuthMethod{ssh.PublicKeys(clientSigner)},
			HostKeyCallback: ssh.FixedHostKey(hostKey),
			Timeout:         5 * time.Second,
		},
	})
	t.Cleanup(func() { _ = ex.Close() })

	// LookPath runs on the target, over SSH.
	bash, err := ex.LookPath(ctx, "bash")
	if err != nil {
		t.Skip("bash not available on this host; skipping wait_until SSH tests")
	}

	dir := t.TempDir()
	h := &waitHarness{
		ex:         ex,
		bash:       bash,
		dir:        dir,
		scriptPath: filepath.Join(dir, waitUntilScriptName),
		countFile:  filepath.Join(dir, "count"),
	}

	is.NoErr(ex.WriteFile(ctx, h.scriptPath, []byte(waitProbeBody(body, h.countFile)), 0o755))
	is.NoErr(ex.Chmod(ctx, h.scriptPath, 0o755))

	return h
}

func TestWaitForScriptOverSSHSucceedsAfterNAttempts(t *testing.T) {
	is := is.New(t)
	h := newWaitSSHHarness(t, countingProbe(3))

	out, err := waitForScript(context.Background(), h.ex,
		h.params(30*time.Second, 20*time.Millisecond, 0, waitUntilRetainLogs))

	is.NoErr(err)
	is.Equal(out.Attempts, 3)
	is.Equal(out.LastKind, probeReady)
	is.True(strings.Contains(out.Last.Stdout, "attempt 3"))
}

// TestWaitForScriptOverSSHNotReadyIsRetried is the regression guard for the
// remote exit-code path: golang.org/x/crypto/ssh reports a non-zero exit as an
// *ssh.ExitError, and it must be classified as "not ready yet" and retried, not
// as an execution error (which fails fast on the first attempt).
func TestWaitForScriptOverSSHNotReadyIsRetried(t *testing.T) {
	is := is.New(t)
	h := newWaitSSHHarness(t, `#!/usr/bin/env bash
echo "still waiting"
echo "nope" >&2
exit 7
`)

	out, err := waitForScript(context.Background(), h.ex,
		h.params(300*time.Millisecond, 20*time.Millisecond, 0, waitUntilRetainLogs))

	is.True(errors.Is(err, errWaitTimeout))
	// The whole point: it kept retrying rather than aborting on attempt 1.
	is.True(out.Attempts >= 2)
	// Assert on the last COMPLETED attempt: the one in flight at the deadline may
	// legitimately be abandoned (see TestWaitForScriptTimesOut).
	// This is the regression guard: an *ssh.ExitError must classify as not-ready.
	is.Equal(classifyProbe(out.LastCompleted), probeNotReady)
	is.Equal(out.LastCompleted.ExitCode, 7)
	is.True(strings.Contains(out.LastCompleted.Stderr, "nope"))
}

// TestWaitForScriptOverSSHExecErrorFailsFast checks that a genuinely unrunnable
// probe is still distinguished from "not ready" over SSH. A remote shell reports
// "command not found" as exit 127, so unlike the local case this surfaces as a
// retried not-ready rather than an immediate abort — assert the loop stays
// well-behaved and reports the shell's diagnostic.
func TestWaitForScriptOverSSHMissingInterpreter(t *testing.T) {
	is := is.New(t)
	h := newWaitSSHHarness(t, `#!/usr/bin/env bash
exit 0
`)

	p := h.params(200*time.Millisecond, 20*time.Millisecond, 0, waitUntilRetainLogs)
	p.Argv = waitUntilArgv([]string{filepath.Join(h.dir, "no-such-interpreter")}, h.scriptPath)

	out, err := waitForScript(context.Background(), h.ex, p)

	is.True(err != nil)
	// As elsewhere, assert on the last COMPLETED attempt: the one in flight when
	// the deadline arrives may legitimately be abandoned.
	is.Equal(classifyProbe(out.LastCompleted), probeNotReady)
	is.Equal(out.LastCompleted.ExitCode, 127)
	is.True(strings.Contains(out.LastCompleted.Stderr, "not found"))
}
