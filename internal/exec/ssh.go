package exec

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/cmd"
	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/cmd/result"
)

// remotePATH is prepended to PATH for the small probe commands the executor
// itself runs (command -v, kill, test, readlink, setsid). Non-interactive SSH
// sessions often have a minimal PATH that omits sbin directories where tools
// like `ip` live; this makes tool discovery and process control reliable
// regardless of the remote login shell configuration. Commands issued by
// resources always use absolute paths (resolved via LookPath at Configure
// time), so they do not depend on this.
const remotePATH = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

// SSHParams configures a SSHExecutor.
type SSHParams struct {
	Addr         string // host:port
	ClientConfig *ssh.ClientConfig
	UseSudo      bool
}

// SSHExecutor runs operations on a remote host over SSH. Commands run through
// ssh sessions; the filesystem is accessed via SFTP; processes are controlled
// with kill/readlink over /proc; sockets are reached with SSH streamlocal
// (unix) or direct-tcpip (tcp) forwarding.
type SSHExecutor struct {
	addr    string
	cfg     *ssh.ClientConfig
	useSudo bool

	mu       sync.Mutex
	client   *ssh.Client
	sftp     *sftp.Client
	done     chan struct{} // closed by Close to stop the keepalive goroutine
	detached string        // "setsid" or "nohup", detected once

	selfPath string // path to the provider binary on the target
}

// Ensure SSHExecutor satisfies the Executor interface.
var _ Executor = (*SSHExecutor)(nil)

// NewSSH creates a SSHExecutor. The connection is established lazily on first
// use. The remote provider-binary path (SelfPath) is set separately via
// SetSelfPath after it has been bootstrapped on the target.
func NewSSH(p SSHParams) *SSHExecutor {
	return &SSHExecutor{
		addr:    p.Addr,
		cfg:     p.ClientConfig,
		useSudo: p.UseSudo,
		done:    make(chan struct{}),
	}
}

func (e *SSHExecutor) IsLocal() bool { return false }

func (e *SSHExecutor) SelfPath() string { return e.selfPath }

// SetSelfPath records the path of the provider binary on the target. It is set
// by the provider after bootstrapping the binary onto the remote host.
func (e *SSHExecutor) SetSelfPath(p string) { e.selfPath = p }

// conn lazily establishes the SSH connection and SFTP client.
func (e *SSHExecutor) conn(_ context.Context) (*ssh.Client, *sftp.Client, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.client != nil && e.sftp != nil {
		return e.client, e.sftp, nil
	}

	client, err := ssh.Dial("tcp", e.addr, e.cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("ssh dial %s: %w", e.addr, err)
	}

	sc, err := sftp.NewClient(client)
	if err != nil {
		client.Close()
		return nil, nil, fmt.Errorf("sftp client: %w", err)
	}

	e.client = client
	e.sftp = sc

	// Keepalive so long idle periods (e.g. during a multi-minute QEMU install)
	// don't get the connection dropped by an idle timeout or NAT.
	go e.keepalive(client)

	return client, sc, nil
}

func (e *SSHExecutor) keepalive(client *ssh.Client) {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-e.done:
			return
		case <-t.C:
			if _, _, err := client.SendRequest("keepalive@openssh.com", true, nil); err != nil {
				return
			}
		}
	}
}

func (e *SSHExecutor) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	select {
	case <-e.done:
	default:
		close(e.done)
	}
	var err error
	if e.sftp != nil {
		err = e.sftp.Close()
		e.sftp = nil
	}
	if e.client != nil {
		if cerr := e.client.Close(); err == nil {
			err = cerr
		}
		e.client = nil
	}
	return err
}

// --- low-level command helpers ---

// runCaptured runs a short command on the target and returns its stdout, exit
// code and any transport error. It does NOT create log files (used for probe
// commands like kill/test/readlink/command -v). PATH is augmented so standard
// tools are found regardless of the login shell.
func (e *SSHExecutor) runCaptured(ctx context.Context, cmdStr string) (string, int, error) {
	client, _, err := e.conn(ctx)
	if err != nil {
		return "", -1, err
	}
	sess, err := client.NewSession()
	if err != nil {
		return "", -1, err
	}
	defer sess.Close()

	var stdout, stderr bytes.Buffer
	sess.Stdout = &stdout
	sess.Stderr = &stderr

	full := fmt.Sprintf("PATH=%q:$PATH %s", remotePATH, cmdStr)
	if runErr := sess.Run(full); runErr != nil {
		if ee, ok := runErr.(*ssh.ExitError); ok {
			return stdout.String(), ee.ExitStatus(), nil
		}
		return stdout.String(), -1, runErr
	}
	return stdout.String(), 0, nil
}

// detachCmd returns the detach helper to use for RunDetached ("setsid" or, if
// unavailable, "nohup"), detected once.
func (e *SSHExecutor) detachCmd(ctx context.Context) string {
	e.mu.Lock()
	d := e.detached
	e.mu.Unlock()
	if d != "" {
		return d
	}
	d = "nohup"
	if _, code, err := e.runCaptured(ctx, "command -v setsid"); err == nil && code == 0 {
		d = "setsid"
	}
	e.mu.Lock()
	e.detached = d
	e.mu.Unlock()
	return d
}

// --- command execution (mirrors internal/cmd, on the target) ---

func (e *SSHExecutor) Run(ctx context.Context, logPath, command string, args ...string) (result.Result, error) {
	res := result.Result{Cmd: command, Args: args}

	client, sc, err := e.conn(ctx)
	if err != nil {
		res.Error = err
		return res, res.Error
	}

	outPath, errPath, err := e.prepareLogs(sc, logPath, command, args)
	if err != nil {
		res.Error = err
		return res, res.Error
	}
	res.Logs.Stdout = outPath
	res.Logs.Stderr = errPath

	outF, err := sc.Create(outPath)
	if err != nil {
		res.Error = fmt.Errorf("failed to create stdout file: %w", err)
		return res, res.Error
	}
	defer outF.Close()
	errF, err := sc.Create(errPath)
	if err != nil {
		res.Error = fmt.Errorf("failed to create stderr file: %w", err)
		return res, res.Error
	}
	defer errF.Close()

	sess, err := client.NewSession()
	if err != nil {
		res.Error = err
		return res, res.Error
	}
	defer sess.Close()

	var outBuf, errBuf lockedBuffer
	sess.Stdout = io.MultiWriter(outF, &outBuf)
	sess.Stderr = io.MultiWriter(errF, &errBuf)

	if runErr := sess.Run(shellJoin(command, args)); runErr != nil {
		if ee, ok := runErr.(*ssh.ExitError); ok {
			res.ExitCode = ee.ExitStatus()
		}
		res.Error = runErr
	}
	res.Stdout = outBuf.String()
	res.Stderr = errBuf.String()

	return res, res.Error
}

func (e *SSHExecutor) RunBG(ctx context.Context, logPath, command string, args ...string) <-chan result.Result {
	ch := make(chan result.Result, 1)
	go func() {
		r, _ := e.Run(ctx, logPath, command, args...)
		ch <- r
		close(ch)
	}()
	return ch
}

func (e *SSHExecutor) RunDetached(ctx context.Context, logPath, command string, args ...string) (result.Result, error) {
	res := result.Result{Cmd: command, Args: args}

	_, sc, err := e.conn(ctx)
	if err != nil {
		res.Error = err
		return res, res.Error
	}

	outPath, errPath, err := e.prepareLogs(sc, logPath, command, args)
	if err != nil {
		res.Error = err
		return res, res.Error
	}
	res.Logs.Stdout = outPath
	res.Logs.Stderr = errPath

	// Launch the process detached from the SSH session's process group so it
	// survives the session (and the provider) exiting, redirect its stdio to
	// the on-target log files, and print its PID. No PTY is requested, which
	// keeps SIGHUP from reaching the child when the session closes.
	inner := shellJoin(command, args)
	wrapper := fmt.Sprintf("PATH=%q:$PATH %s %s >%s 2>%s </dev/null & echo $!",
		remotePATH, e.detachCmd(ctx), inner, shellQuoteArg(outPath), shellQuoteArg(errPath))

	stdout, code, runErr := e.runCaptured(ctx, wrapper)
	if runErr != nil {
		res.Error = runErr
		return res, res.Error
	}
	if code != 0 {
		res.ExitCode = code
		res.Error = fmt.Errorf("failed to start detached command (exit %d)", code)
		return res, res.Error
	}
	if pid, perr := strconv.Atoi(strings.TrimSpace(stdout)); perr == nil {
		res.PID = pid
	}

	// Give the command a moment to write any early output, matching the local
	// RunDetached behavior.
	<-time.After(250 * time.Millisecond)

	return res, res.Error
}

// prepareLogs creates the log directory and the command log file on the target
// and returns the stdout/stderr log paths, mirroring internal/cmd naming.
func (e *SSHExecutor) prepareLogs(sc *sftp.Client, logPath, command string, args []string) (string, string, error) {
	if err := sc.MkdirAll(logPath); err != nil {
		return "", "", fmt.Errorf("failed to create log directory: %w", err)
	}
	cmdName := path.Base(command)
	ts := cmd.Now().Format("20060102_150405")
	outPath := path.Join(logPath, fmt.Sprintf("%s_%s_stdout.log", ts, cmdName))
	errPath := path.Join(logPath, fmt.Sprintf("%s_%s_stderr.log", ts, cmdName))
	cmdPath := path.Join(logPath, fmt.Sprintf("%s_%s_command.log", ts, cmdName))

	if err := e.writeFile(sc, cmdPath, []byte(fmt.Sprintf("command=%s args=%v\n", command, args)), 0o600); err != nil {
		return "", "", fmt.Errorf("failed to create command log file: %w", err)
	}
	return outPath, errPath, nil
}

// --- filesystem (via SFTP, on the target) ---

func (e *SSHExecutor) MkdirAll(ctx context.Context, p string, perm os.FileMode) error {
	_, sc, err := e.conn(ctx)
	if err != nil {
		return err
	}
	if err := sc.MkdirAll(p); err != nil {
		return err
	}
	return sc.Chmod(p, perm)
}

func (e *SSHExecutor) WriteFile(ctx context.Context, p string, data []byte, perm os.FileMode) error {
	_, sc, err := e.conn(ctx)
	if err != nil {
		return err
	}
	return e.writeFile(sc, p, data, perm)
}

func (e *SSHExecutor) writeFile(sc *sftp.Client, p string, data []byte, perm os.FileMode) error {
	f, err := sc.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return sc.Chmod(p, perm)
}

func (e *SSHExecutor) ReadFile(ctx context.Context, p string) ([]byte, error) {
	_, sc, err := e.conn(ctx)
	if err != nil {
		return nil, err
	}
	f, err := sc.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

func (e *SSHExecutor) Remove(ctx context.Context, p string) error {
	_, sc, err := e.conn(ctx)
	if err != nil {
		return err
	}
	return removeAll(sc, p)
}

// removeAll mirrors os.RemoveAll over SFTP (which has no recursive delete).
func removeAll(sc *sftp.Client, p string) error {
	fi, err := sc.Lstat(p)
	if err != nil {
		if errIsNotExist(err) {
			return nil
		}
		return err
	}
	if fi.IsDir() {
		entries, err := sc.ReadDir(p)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := removeAll(sc, path.Join(p, entry.Name())); err != nil {
				return err
			}
		}
		return sc.RemoveDirectory(p)
	}
	return sc.Remove(p)
}

func (e *SSHExecutor) Rename(ctx context.Context, oldpath, newpath string) error {
	_, sc, err := e.conn(ctx)
	if err != nil {
		return err
	}
	// PosixRename overwrites an existing destination, matching os.Rename.
	return sc.PosixRename(oldpath, newpath)
}

func (e *SSHExecutor) Stat(ctx context.Context, p string) (os.FileInfo, error) {
	_, sc, err := e.conn(ctx)
	if err != nil {
		return nil, err
	}
	return sc.Stat(p)
}

func (e *SSHExecutor) Chmod(ctx context.Context, p string, mode os.FileMode) error {
	_, sc, err := e.conn(ctx)
	if err != nil {
		return err
	}
	return sc.Chmod(p, mode)
}

func (e *SSHExecutor) ReadDir(ctx context.Context, p string) ([]os.DirEntry, error) {
	_, sc, err := e.conn(ctx)
	if err != nil {
		return nil, err
	}
	infos, err := sc.ReadDir(p)
	if err != nil {
		return nil, err
	}
	entries := make([]os.DirEntry, len(infos))
	for i, fi := range infos {
		entries[i] = fs.FileInfoToDirEntry(fi)
	}
	return entries, nil
}

func (e *SSHExecutor) OpenWrite(ctx context.Context, p string, perm os.FileMode) (io.WriteCloser, error) {
	_, sc, err := e.conn(ctx)
	if err != nil {
		return nil, err
	}
	f, err := sc.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return nil, err
	}
	if err := sc.Chmod(p, perm); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

func (e *SSHExecutor) OpenRead(ctx context.Context, p string) (io.ReadCloser, error) {
	_, sc, err := e.conn(ctx)
	if err != nil {
		return nil, err
	}
	return sc.Open(p)
}

func (e *SSHExecutor) CopyFile(ctx context.Context, src, dst string) (int64, error) {
	_, sc, err := e.conn(ctx)
	if err != nil {
		return 0, err
	}
	in, err := sc.Open(src)
	if err != nil {
		return 0, err
	}
	defer in.Close()
	out, err := sc.Create(dst)
	if err != nil {
		return 0, err
	}
	defer out.Close()
	return io.Copy(out, in)
}

func (e *SSHExecutor) Upload(ctx context.Context, localPath, targetPath string, perm os.FileMode) (int64, error) {
	_, sc, err := e.conn(ctx)
	if err != nil {
		return 0, err
	}
	in, err := os.Open(localPath)
	if err != nil {
		return 0, err
	}
	defer in.Close()
	out, err := sc.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return 0, err
	}
	n, copyErr := io.Copy(out, in)
	if cerr := out.Close(); copyErr == nil {
		copyErr = cerr
	}
	if copyErr != nil {
		return n, copyErr
	}
	return n, sc.Chmod(targetPath, perm)
}

// --- process management (on the target) ---

func (e *SSHExecutor) IsRunning(ctx context.Context, pid int, expectedExe string) (bool, error) {
	// Existence is checked via /proc, which is ownership-independent (works even
	// when the process is owned by root because it was started with sudo). A
	// process in the zombie/defunct state still has a /proc entry but has
	// terminated, so it is treated as not running.
	_, code, err := e.runCaptured(ctx, fmt.Sprintf("test -d /proc/%d && ! grep -qs '^State:[[:space:]]*[Zz]' /proc/%d/status", pid, pid))
	if err != nil {
		return false, err
	}
	if code != 0 {
		return false, nil
	}
	if expectedExe != "" {
		readlink := fmt.Sprintf("readlink /proc/%d/exe", pid)
		if e.useSudo {
			readlink = "sudo -n " + readlink
		}
		out, code, err := e.runCaptured(ctx, readlink)
		if err != nil {
			return false, err
		}
		if code != 0 {
			return false, nil
		}
		if !strings.EqualFold(strings.TrimSpace(out), expectedExe) {
			return false, nil
		}
	}
	return true, nil
}

func (e *SSHExecutor) Kill(ctx context.Context, pid int, sig os.Signal) error {
	signum := signalNumber(sig)
	killCmd := fmt.Sprintf("kill -%d %d", signum, pid)
	if e.useSudo {
		killCmd = fmt.Sprintf("sudo -n kill -%d %d", signum, pid)
	}
	out, code, err := e.runCaptured(ctx, killCmd)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("kill -%d %d failed (exit %d): %s", signum, pid, code, strings.TrimSpace(out))
	}
	return nil
}

// --- sockets ---

func (e *SSHExecutor) Dial(ctx context.Context, network, addr string, timeout time.Duration) (net.Conn, error) {
	client, _, err := e.conn(ctx)
	if err != nil {
		return nil, err
	}

	type dialResult struct {
		c   net.Conn
		err error
	}
	ch := make(chan dialResult, 1)
	go func() {
		c, err := client.Dial(network, addr)
		ch <- dialResult{c, err}
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		go func() {
			if r := <-ch; r.c != nil {
				r.c.Close()
			}
		}()
		return nil, ctx.Err()
	case <-timer.C:
		go func() {
			if r := <-ch; r.c != nil {
				r.c.Close()
			}
		}()
		return nil, fmt.Errorf("dial %s %s: timeout after %s", network, addr, timeout)
	case r := <-ch:
		return r.c, r.err
	}
}

// --- misc ---

func (e *SSHExecutor) LookPath(ctx context.Context, file string) (string, error) {
	out, code, err := e.runCaptured(ctx, "command -v "+shellQuoteArg(file))
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", fmt.Errorf("%s: executable file not found on target", file)
	}
	p := strings.TrimSpace(out)
	if p == "" {
		return "", fmt.Errorf("%s: executable file not found on target", file)
	}
	return p, nil
}

// --- helpers ---

// shellQuoteArg single-quotes an argument so the remote shell treats it
// literally.
func shellQuoteArg(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// shellJoin builds a single shell command string from a command and its args,
// quoting each so they are passed through unchanged (an ssh session executes a
// single string via the remote login shell, not an argv vector).
func shellJoin(command string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, shellQuoteArg(command))
	for _, a := range args {
		parts = append(parts, shellQuoteArg(a))
	}
	return strings.Join(parts, " ")
}

// errIsNotExist reports whether err is a "not found" error from SFTP.
func errIsNotExist(err error) bool {
	return err != nil && IsNotExist(err)
}

// lockedBuffer is a bytes.Buffer safe for concurrent use.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}
