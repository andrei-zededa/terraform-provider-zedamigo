package exec

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	stdexec "os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/shirou/gopsutil/v4/process"

	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/cmd"
	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/cmd/result"
)

// LocalExecutor runs everything on localhost. Every method delegates to the
// same standard-library / internal/cmd / gopsutil functions the provider used
// before the Executor abstraction existed, so localhost behavior is unchanged.
type LocalExecutor struct {
	useSudo bool

	sudoOnce sync.Once
	sudoPath string
}

// Ensure LocalExecutor satisfies the Executor interface.
var _ Executor = (*LocalExecutor)(nil)

// NewLocal creates a LocalExecutor. useSudo mirrors the provider's use_sudo
// option and only affects Kill (the only operation that internalizes the sudo
// decision; command sudo prefixes are still assembled by the callers).
func NewLocal(useSudo bool) *LocalExecutor {
	return &LocalExecutor{useSudo: useSudo}
}

func (l *LocalExecutor) IsLocal() bool { return true }

func (l *LocalExecutor) SelfPath() string {
	if p, err := os.Executable(); err == nil && p != "" {
		return p
	}
	return os.Args[0]
}

func (l *LocalExecutor) LookPath(_ context.Context, file string) (string, error) {
	return stdexec.LookPath(file)
}

func (l *LocalExecutor) Run(_ context.Context, logPath, command string, args ...string) (result.Result, error) {
	return cmd.Run(logPath, command, args...)
}

func (l *LocalExecutor) RunBG(_ context.Context, logPath, command string, args ...string) <-chan result.Result {
	return cmd.RunBG(logPath, command, args...)
}

func (l *LocalExecutor) RunDetached(_ context.Context, logPath, command string, args ...string) (result.Result, error) {
	return cmd.RunDetached(logPath, command, args...)
}

func (l *LocalExecutor) MkdirAll(_ context.Context, path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

func (l *LocalExecutor) WriteFile(_ context.Context, path string, data []byte, perm os.FileMode) error {
	return os.WriteFile(path, data, perm)
}

func (l *LocalExecutor) ReadFile(_ context.Context, path string) ([]byte, error) {
	return os.ReadFile(path)
}

func (l *LocalExecutor) Remove(_ context.Context, path string) error {
	return os.RemoveAll(path)
}

func (l *LocalExecutor) Rename(_ context.Context, oldpath, newpath string) error {
	return os.Rename(oldpath, newpath)
}

func (l *LocalExecutor) Stat(_ context.Context, path string) (os.FileInfo, error) {
	return os.Stat(path)
}

func (l *LocalExecutor) Chmod(_ context.Context, path string, mode os.FileMode) error {
	return os.Chmod(path, mode)
}

func (l *LocalExecutor) ReadDir(_ context.Context, path string) ([]os.DirEntry, error) {
	return os.ReadDir(path)
}

func (l *LocalExecutor) OpenWrite(_ context.Context, path string, perm os.FileMode) (io.WriteCloser, error) {
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
}

func (l *LocalExecutor) OpenRead(_ context.Context, path string) (io.ReadCloser, error) {
	return os.Open(path)
}

func (l *LocalExecutor) CopyFile(_ context.Context, src, dst string) (int64, error) {
	return cmd.CopyFile(src, dst)
}

func (l *LocalExecutor) Upload(_ context.Context, localPath, targetPath string, perm os.FileMode) (int64, error) {
	n, err := cmd.CopyFile(localPath, targetPath)
	if err != nil {
		return n, err
	}
	return n, os.Chmod(targetPath, perm)
}

func (l *LocalExecutor) IsRunning(_ context.Context, pid int, expectedExe string) (bool, error) {
	p, err := process.NewProcess(int32(pid))
	if err != nil {
		// The process does not exist (or can't be inspected); treat as not
		// running rather than an error so callers can self-heal.
		return false, nil
	}
	running, err := p.IsRunning()
	if err != nil {
		return false, err
	}
	if !running {
		return false, nil
	}
	if expectedExe != "" {
		exe, err := p.Exe()
		if err != nil {
			return false, err
		}
		if !strings.EqualFold(exe, expectedExe) {
			return false, nil
		}
	}
	return true, nil
}

func (l *LocalExecutor) Kill(_ context.Context, pid int, sig os.Signal) error {
	if l.useSudo {
		signum := signalNumber(sig)
		c := stdexec.Command(l.sudo(), "-n", "kill", fmt.Sprintf("-%d", signum), strconv.Itoa(pid))
		out, err := c.CombinedOutput()
		if err != nil {
			return fmt.Errorf("sudo kill -%d %d failed: %w: %s", signum, pid, err, strings.TrimSpace(string(out)))
		}
		return nil
	}

	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Signal(sig)
}

func (l *LocalExecutor) Dial(ctx context.Context, network, addr string, timeout time.Duration) (net.Conn, error) {
	d := net.Dialer{Timeout: timeout}
	return d.DialContext(ctx, network, addr)
}

func (l *LocalExecutor) Close() error { return nil }

// sudo lazily resolves the absolute path to the sudo executable. The provider
// validates that sudo exists when use_sudo is set, so this should not fail in
// practice; fall back to the bare name to rely on PATH if lookup fails.
func (l *LocalExecutor) sudo() string {
	l.sudoOnce.Do(func() {
		if p, err := stdexec.LookPath("sudo"); err == nil {
			l.sudoPath = p
		} else {
			l.sudoPath = "sudo"
		}
	})
	return l.sudoPath
}

// signalNumber returns the numeric value of a signal for use with the `kill`
// command. Defaults to SIGKILL (9) for signals that aren't a syscall.Signal.
func signalNumber(sig os.Signal) int {
	if s, ok := sig.(syscall.Signal); ok {
		return int(s)
	}
	return int(syscall.SIGKILL)
}
