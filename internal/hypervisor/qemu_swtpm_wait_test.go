// SPDX-License-Identifier: MPL-2.0

package hypervisor

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/matryer/is"

	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/exec"
)

// staleSocketFile creates a unix socket file with no listener behind it, the
// exact state a swtpm control socket is in after the swtpm process exited:
// the file exists but connections are refused.
func staleSocketFile(t *testing.T, path string) {
	t.Helper()
	is := is.New(t)
	fd, err := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	is.NoErr(err)
	is.NoErr(syscall.Bind(fd, &syscall.SockaddrUnix{Name: path}))
	is.NoErr(syscall.Close(fd))
}

func TestWaitForSwTPMSocketImmediate(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	sock := filepath.Join(t.TempDir(), "swtpm.socket")

	l, err := net.Listen("unix", sock)
	is.NoErr(err)
	defer l.Close()

	is.NoErr(WaitForSwTPMSocket(ctx, exec.NewLocal(false), sock, 2*time.Second))
}

// TestWaitForSwTPMSocketRestartWindow simulates the swtpm restart window: the
// stale socket file of the exited swtpm refuses connections until the process
// monitor restarts swtpm (here: a listener that appears after a delay).
func TestWaitForSwTPMSocketRestartWindow(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	sock := filepath.Join(t.TempDir(), "swtpm.socket")

	staleSocketFile(t, sock)

	go func() {
		time.Sleep(1 * time.Second) // the monitor's restart delay
		// swtpm re-binds its socket path on restart.
		os.Remove(sock)
		l, err := net.Listen("unix", sock)
		if err == nil {
			t.Cleanup(func() { l.Close() })
		}
	}()

	start := time.Now()
	is.NoErr(WaitForSwTPMSocket(ctx, exec.NewLocal(false), sock, 5*time.Second))
	is.True(time.Since(start) >= 900*time.Millisecond) // it must have actually waited for the restart
}

func TestWaitForSwTPMSocketTimeout(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	sock := filepath.Join(t.TempDir(), "swtpm.socket")

	staleSocketFile(t, sock)

	err := WaitForSwTPMSocket(ctx, exec.NewLocal(false), sock, 1*time.Second)
	is.True(err != nil) // must fail when nothing ever listens
	is.True(strings.Contains(err.Error(), "did not accept a connection"))
	is.True(strings.Contains(err.Error(), "zedamigo_swtpm"))
}

func TestWaitForSwTPMSocketContextCanceled(t *testing.T) {
	is := is.New(t)
	sock := filepath.Join(t.TempDir(), "swtpm.socket")

	staleSocketFile(t, sock)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(300 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	err := WaitForSwTPMSocket(ctx, exec.NewLocal(false), sock, 30*time.Second)
	is.True(err != nil)                        // canceled, not successful
	is.Equal(err, context.Canceled)            // and with the context's error
	is.True(time.Since(start) < 5*time.Second) // well before the 30s timeout
}
