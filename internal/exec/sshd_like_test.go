package exec_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// agentChannelType is the client-bound channel sshd opens to reach the forwarded
// agent.
const agentChannelType = "auth-agent@openssh.com"

// sshdLike is a minimal SSH server that models the two OpenSSH behaviors
// gliderlabs/ssh does not, both of which decide whether agent forwarding
// actually works:
//
//  1. an auth-agent-req@openssh.com is honored only ONCE per connection — sshd
//     fails every later request on the same connection ("authentication
//     forwarding requested twice"); and
//  2. the agent socket it sets up belongs to the CONNECTION, not to the session
//     that asked for it, so it outlives that session and every later session
//     inherits SSH_AUTH_SOCK from it.
//
// gliderlabs/ssh instead replies true to every request, which is why a
// per-session implementation looked correct in tests while failing against a real
// sshd from the second command onwards.
//
// It implements only what SSHExecutor needs: session channels, "exec", and the
// sftp subsystem (Run writes its log files over SFTP).
type sshdLike struct {
	Addr    string
	HostKey ssh.PublicKey

	// AgentReqs counts every auth-agent-req seen, across all connections.
	AgentReqs atomic.Int32

	denyAgent bool   // model AllowAgentForwarding=no: refuse every request
	sockDir   string // pre-created so no test helper is called off the test goroutine

	mu       sync.Mutex
	closers  []io.Closer
	sockSeq  int
	shutdown bool
}

type sshdLikeOpts struct {
	denyAgent bool
}

func newSSHDLike(t *testing.T, authorized ssh.PublicKey, opts sshdLikeOpts) *sshdLike {
	t.Helper()

	hostSigner := genSigner(t)
	s := &sshdLike{
		HostKey:   hostSigner.PublicKey(),
		denyAgent: opts.denyAgent,
		sockDir:   t.TempDir(),
	}

	cfg := &ssh.ServerConfig{
		PublicKeyCallback: func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if bytes.Equal(key.Marshal(), authorized.Marshal()) {
				return &ssh.Permissions{}, nil
			}
			return nil, fmt.Errorf("unauthorized key")
		},
	}
	cfg.AddHostKey(hostSigner)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	s.Addr = ln.Addr().String()
	s.track(ln)

	go func() {
		for {
			nc, err := ln.Accept()
			if err != nil {
				return
			}
			go s.serveConn(nc, cfg)
		}
	}()

	t.Cleanup(s.close)

	return s
}

// track registers a closer to be shut down with the server. Listeners are opened
// from connection goroutines, so they cannot each register their own t.Cleanup.
func (s *sshdLike) track(c io.Closer) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.shutdown {
		_ = c.Close()
		return
	}
	s.closers = append(s.closers, c)
}

func (s *sshdLike) close() {
	s.mu.Lock()
	s.shutdown = true
	closers := s.closers
	s.closers = nil
	s.mu.Unlock()

	for _, c := range closers {
		_ = c.Close()
	}
}

// nextSockPath returns a fresh path for a forwarded-agent socket. Unix socket
// paths are length-limited, so keep it short.
func (s *sshdLike) nextSockPath() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.sockSeq++

	return filepath.Join(s.sockDir, fmt.Sprintf("agent.%d", s.sockSeq))
}

func (s *sshdLike) serveConn(nc net.Conn, cfg *ssh.ServerConfig) {
	sconn, chans, reqs, err := ssh.NewServerConn(nc, cfg)
	if err != nil {
		return
	}
	defer sconn.Close()
	go ssh.DiscardRequests(reqs)

	// Agent state lives on the CONNECTION, exactly where sshd keeps it.
	var (
		mu        sync.Mutex
		agentSock string
		agentDone bool
	)

	for nch := range chans {
		if nch.ChannelType() != "session" {
			_ = nch.Reject(ssh.UnknownChannelType, "only session channels")
			continue
		}
		ch, chReqs, err := nch.Accept()
		if err != nil {
			continue
		}

		go func() {
			defer ch.Close()

			for req := range chReqs {
				switch req.Type {
				case "auth-agent-req@openssh.com":
					s.AgentReqs.Add(1)

					mu.Lock()
					// Only the first request on the connection is honored.
					accept := !agentDone && !s.denyAgent
					agentDone = true
					if accept {
						agentSock = s.serveAgentSocket(sconn)
						accept = agentSock != ""
					}
					mu.Unlock()

					if req.WantReply {
						_ = req.Reply(accept, nil)
					}

				case "exec":
					var p struct{ Command string }
					_ = ssh.Unmarshal(req.Payload, &p)
					if req.WantReply {
						_ = req.Reply(true, nil)
					}

					mu.Lock()
					sock := agentSock
					mu.Unlock()

					runExecRequest(ch, p.Command, sock)

					return

				case "subsystem":
					var p struct{ Name string }
					_ = ssh.Unmarshal(req.Payload, &p)
					if p.Name != "sftp" {
						if req.WantReply {
							_ = req.Reply(false, nil)
						}
						continue
					}
					if req.WantReply {
						_ = req.Reply(true, nil)
					}
					if srv, err := sftp.NewServer(ch); err == nil {
						_ = srv.Serve()
					}

					return

				default:
					if req.WantReply {
						_ = req.Reply(false, nil)
					}
				}
			}
		}()
	}
}

// serveAgentSocket creates the connection-scoped unix socket that sshd hands to
// commands as SSH_AUTH_SOCK, forwarding anything that connects to it back to the
// client over an auth-agent channel. It returns "" if the socket can't be set up,
// which is how sshd would fail the request.
func (s *sshdLike) serveAgentSocket(sconn *ssh.ServerConn) string {
	sock := s.nextSockPath()

	ln, err := net.Listen("unix", sock)
	if err != nil {
		return ""
	}
	s.track(ln)

	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go proxyToAgentChannel(sconn, c)
		}
	}()

	return sock
}

func proxyToAgentChannel(sconn *ssh.ServerConn, c net.Conn) {
	defer c.Close()

	ch, reqs, err := sconn.OpenChannel(agentChannelType, nil)
	if err != nil {
		return
	}
	defer ch.Close()
	go ssh.DiscardRequests(reqs)

	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(ch, c)
		_ = ch.CloseWrite()
		close(done)
	}()
	_, _ = io.Copy(c, ch)
	<-done
}

// runExecRequest runs the command the way sshd does, exporting SSH_AUTH_SOCK only
// when this connection actually has agent forwarding set up.
func runExecRequest(ch ssh.Channel, command, agentSock string) {
	c := osexec.Command("/bin/sh", "-c", command)
	c.Stdout = ch
	c.Stderr = ch.Stderr()
	// Drop any SSH_AUTH_SOCK inherited by the test process, so it can never be
	// mistaken for a forwarded one, then add the forwarded socket if there is one.
	c.Env = withoutEnv(os.Environ(), "SSH_AUTH_SOCK")
	if agentSock != "" {
		c.Env = append(c.Env, "SSH_AUTH_SOCK="+agentSock)
	}

	var status uint32
	if err := c.Run(); err != nil {
		var ee *osexec.ExitError
		if errors.As(err, &ee) {
			status = uint32(ee.ExitCode()) //nolint:gosec // exit codes are small non-negative ints
		} else {
			status = 127
		}
	}

	_, _ = ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{status}))
}

func withoutEnv(env []string, key string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		if !strings.HasPrefix(kv, key+"=") {
			out = append(out, kv)
		}
	}

	return out
}
