// Package exec provides an abstraction over every operation that touches the
// "target" environment on which the provider creates resources: running
// commands, manipulating the filesystem, managing processes and dialing
// sockets.
//
// There are two implementations:
//
//   - LocalExecutor runs everything on localhost (the machine running the
//     provider). It delegates to internal/cmd, the os.* family, gopsutil and
//     net.Dial, so its behavior is identical to how the provider worked before
//     this abstraction existed.
//   - SSHExecutor runs everything on a remote host reached over SSH, using
//     golang.org/x/crypto/ssh for commands/processes/sockets and
//     github.com/pkg/sftp for the filesystem.
//
// All paths passed to an Executor are paths ON THE TARGET. The provider plugin
// process always runs locally; only the operations move to the target.
package exec

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net"
	"os"
	"time"

	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/cmd/result"
)

// Executor abstracts the target environment. Every method that can perform I/O
// takes a context so the SSHExecutor can honor cancellation/timeouts; the
// LocalExecutor accepts the context for signature parity and mostly ignores it
// (the underlying internal/cmd functions are not yet context-aware).
type Executor interface {
	// IsLocal reports whether this executor operates on localhost.
	IsLocal() bool

	// SelfPath returns the absolute path to the provider binary ON THE TARGET.
	// It is used by the self-invoked daemons (-dhcp-server, -gvproxy,
	// -socket-tailer, ...). Locally this is the running executable; remotely it
	// is the binary that was bootstrapped onto the target.
	SelfPath() string

	// LookPath resolves an executable name to a path ON THE TARGET.
	LookPath(ctx context.Context, file string) (string, error)

	// --- command execution (mirrors internal/cmd) ---

	// Run executes command+args ON THE TARGET, writing per-invocation
	// stdout/stderr/command log files under logPath (also on the target), and
	// returns the captured output and exit code.
	Run(ctx context.Context, logPath, command string, args ...string) (result.Result, error)
	// RunBG runs Run asynchronously.
	RunBG(ctx context.Context, logPath, command string, args ...string) <-chan result.Result
	// RunDetached starts a long-lived process ON THE TARGET that survives the
	// provider process exiting, capturing its PID into Result.PID.
	RunDetached(ctx context.Context, logPath, command string, args ...string) (result.Result, error)

	// --- filesystem ON THE TARGET (mirror os.*) ---

	MkdirAll(ctx context.Context, path string, perm os.FileMode) error
	WriteFile(ctx context.Context, path string, data []byte, perm os.FileMode) error
	ReadFile(ctx context.Context, path string) ([]byte, error)
	// Remove removes path recursively (os.RemoveAll semantics).
	Remove(ctx context.Context, path string) error
	Rename(ctx context.Context, oldpath, newpath string) error
	Stat(ctx context.Context, path string) (os.FileInfo, error)
	Chmod(ctx context.Context, path string, mode os.FileMode) error
	ReadDir(ctx context.Context, path string) ([]os.DirEntry, error)
	// OpenWrite creates/truncates a file ON THE TARGET and returns a writer to
	// it; used where the caller streams output (e.g. text/template.Execute).
	OpenWrite(ctx context.Context, path string, perm os.FileMode) (io.WriteCloser, error)
	// OpenRead opens a file ON THE TARGET for reading.
	OpenRead(ctx context.Context, path string) (io.ReadCloser, error)
	// CopyFile copies src to dst, both ON THE TARGET.
	CopyFile(ctx context.Context, src, dst string) (int64, error)
	// Upload copies a file from the LOCAL filesystem (the provider host) to the
	// target. On the LocalExecutor this is a plain local copy.
	Upload(ctx context.Context, localPath, targetPath string, perm os.FileMode) (int64, error)

	// --- process management ON THE TARGET ---

	// IsRunning reports whether pid is alive on the target. When expectedExe is
	// non-empty it additionally verifies that the process' executable path
	// matches expectedExe (used to confirm a PID is "ours").
	IsRunning(ctx context.Context, pid int, expectedExe string) (bool, error)
	// Kill sends sig to pid on the target, using sudo when the executor was
	// configured with use_sudo.
	Kill(ctx context.Context, pid int, sig os.Signal) error

	// --- sockets dialed BY the provider process ---

	// Dial connects to a socket ON THE TARGET. For unix sockets on a remote
	// target this tunnels over SSH (streamlocal forwarding). Used by QMP and
	// swtpm.
	Dial(ctx context.Context, network, addr string, timeout time.Duration) (net.Conn, error)

	// Close releases any underlying connection. No-op for the LocalExecutor.
	io.Closer
}

// IsNotExist reports whether err indicates a missing file/directory. It
// normalizes the "not found" errors returned by os.Stat (LocalExecutor) and
// github.com/pkg/sftp (SSHExecutor) so callers can use a single check.
func IsNotExist(err error) bool {
	return errors.Is(err, fs.ErrNotExist)
}
