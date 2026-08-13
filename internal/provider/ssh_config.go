// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net"
	"os"
	osuser "os/user"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/exec"
	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/undent"
)

// SSHModel describes the nested `ssh {}` block on the provider, used when
// `target` is not localhost. Every attribute also has a ZEDAMIGO_SSH_* env
// fallback.
type SSHModel struct {
	User                  types.String `tfsdk:"user"`
	Port                  types.Int64  `tfsdk:"port"`
	Password              types.String `tfsdk:"password"`
	PrivateKey            types.String `tfsdk:"private_key"`
	PrivateKeyFile        types.String `tfsdk:"private_key_file"`
	PrivateKeyPassphrase  types.String `tfsdk:"private_key_passphrase"`
	UseAgent              types.Bool   `tfsdk:"use_agent"`
	ForwardAgent          types.Bool   `tfsdk:"forward_agent"`
	KnownHostsFile        types.String `tfsdk:"known_hosts_file"`
	HostKey               types.String `tfsdk:"host_key"`
	InsecureIgnoreHostKey types.Bool   `tfsdk:"insecure_ignore_host_key"`
	ProxyJump             types.String `tfsdk:"proxy_jump"`
	RemoteBinaryPath      types.String `tfsdk:"remote_binary_path"`
}

// sshSchemaBlock returns the provider-level `ssh {}` block schema.
func sshSchemaBlock() schema.Block {
	return schema.SingleNestedBlock{
		Description: "SSH connection settings used when `target` is a remote host (anything other than `localhost`).",
		MarkdownDescription: undent.Md(`
		SSH connection settings used when |target| is a remote host (anything other than
		|localhost|). All attributes are optional and each has a |ZEDAMIGO_SSH_*| environment
		variable fallback. Provide at least one authentication method: |password|,
		|private_key|/|private_key_file|, or |use_agent|.

		NOTE: the target's sshd |MaxSessions| bounds the |-parallelism| you can apply with. Everything
		the provider does on a remote target goes over ONE SSH connection: each command is a new session
		channel on it and SFTP permanently holds one more. OpenSSH counts those against |MaxSessions|
		(default 10) per connection and refuses the ones over the limit, which surfaces as a resource
		failing with:

		    ssh: rejected: connect failed (open failed)

		It hits whichever resource loses the race, so it looks arbitrary, and the same command run by
		hand on the target works fine. A config that creates many host-networking resources at once —
		|zedamigo_netns| / |zedamigo_tap| / |zedamigo_dhcp_server| for a multi-port edge node, say — is
		what reaches it. Either apply with a lower |-parallelism| (leave a couple of sessions of
		headroom under the target's limit) or raise |MaxSessions| there. Jump hosts do not enter into
		it: they carry a single |direct-tcpip| channel, which is not counted.

		The target's effective limit is the |maxsessions| line of |sshd -T|; sshd logs |no more sessions|
		when it refuses one.`),
		Attributes: map[string]schema.Attribute{
			"user": schema.StringAttribute{
				Description: "SSH username. Defaults to the current local user. Env: ZEDAMIGO_SSH_USER.",
				Optional:    true,
			},
			"port": schema.Int64Attribute{
				Description: "SSH port. Defaults to 22. Env: ZEDAMIGO_SSH_PORT.",
				Optional:    true,
			},
			"password": schema.StringAttribute{
				Description: "SSH password. Env: ZEDAMIGO_SSH_PASSWORD.",
				Optional:    true,
				Sensitive:   true,
			},
			"private_key": schema.StringAttribute{
				Description: "SSH private key material (PEM). Env: ZEDAMIGO_SSH_PRIVATE_KEY.",
				Optional:    true,
				Sensitive:   true,
			},
			"private_key_file": schema.StringAttribute{
				Description: "Path to a SSH private key file (read locally). Env: ZEDAMIGO_SSH_PRIVATE_KEY_FILE.",
				Optional:    true,
			},
			"private_key_passphrase": schema.StringAttribute{
				Description: "Passphrase for an encrypted private key. Env: ZEDAMIGO_SSH_PRIVATE_KEY_PASSPHRASE.",
				Optional:    true,
				Sensitive:   true,
			},
			"use_agent": schema.BoolAttribute{
				Description: "Use the SSH agent at $SSH_AUTH_SOCK for public-key auth. Env: ZEDAMIGO_SSH_USE_AGENT.",
				Optional:    true,
			},
			"forward_agent": schema.BoolAttribute{
				Description: "Forward the local SSH agent at $SSH_AUTH_SOCK to the target, so commands run there " +
					"(e.g. a zedamigo_wait_until script sshing on to an edge node) can authenticate with your keys " +
					"without copying them to the target. Default false. Env: ZEDAMIGO_SSH_FORWARD_AGENT.",
				MarkdownDescription: undent.Md(`
				Forward the local SSH agent at |$SSH_AUTH_SOCK| to the target, the equivalent of OpenSSH's
				|ForwardAgent yes| / |ssh -A|. Commands the provider runs on the target then get an |SSH_AUTH_SOCK|
				of their own and can authenticate onward with the operator's keys, without any private key ever
				being copied to the target. The typical use is a |zedamigo_wait_until| script that sshes from the
				target on to an edge node.

				Defaults to |false|. Independent of |use_agent|, which controls whether the agent is used to
				authenticate TO the target: either can be set without the other, though both need a running agent.

				Only the target receives the agent — jump hosts merely tunnel the connection.

				Forwarding is negotiated once, when the provider connects, and then applies to every command it runs
				(sshd allows the request only once per connection and binds the agent socket to the connection, not to
				a single session). The target's sshd must permit it — |AllowAgentForwarding yes|, the OpenSSH default,
				and no |no-agent-forwarding| restriction on the key in |authorized_keys| — or configuring the provider
				fails with an explicit error.

				Long-lived daemons the provider starts on the target (DHCP/RA servers and the like) do inherit the
				socket, but it stops working once the provider exits and its SSH connection closes, so do not rely on
				it there.

				SECURITY: anyone able to read the forwarded socket on the target — root, or the same user — can use
				your loaded keys for as long as the command runs. Only enable it for targets you trust, and prefer
				agent keys that are confirmation- or lifetime-constrained (|ssh-add -c|, |ssh-add -t|).
				Env: |ZEDAMIGO_SSH_FORWARD_AGENT|.`),
				Optional: true,
			},
			"known_hosts_file": schema.StringAttribute{
				Description: "Path to a known_hosts file for host key verification. Defaults to ~/.ssh/known_hosts if present. Env: ZEDAMIGO_SSH_KNOWN_HOSTS.",
				Optional:    true,
			},
			"host_key": schema.StringAttribute{
				Description: "Pinned host public key in authorized_keys line format. Env: ZEDAMIGO_SSH_HOST_KEY.",
				Optional:    true,
			},
			"insecure_ignore_host_key": schema.BoolAttribute{
				Description: "Skip host key verification (INSECURE; dev/test only). Env: ZEDAMIGO_SSH_INSECURE.",
				Optional:    true,
			},
			"proxy_jump": schema.StringAttribute{
				Description: "Jump/bastion host(s) to tunnel the connection through, in OpenSSH ProxyJump format: [user@]host[:port], comma-separated for a chain (e.g. \"root@localhost:11022\"). Jump hosts reuse the target's authentication (password/keys/agent) and, for host key verification, its known_hosts_file or insecure_ignore_host_key (host_key applies only to the final target). Env: ZEDAMIGO_SSH_PROXY_JUMP.",
				Optional:    true,
			},
			"remote_binary_path": schema.StringAttribute{
				Description: "Path to the provider binary on the remote host (used by self-invoked daemons). If unset, the binary is bootstrapped via the install script for the provider's version. Env: ZEDAMIGO_REMOTE_BINARY_PATH.",
				Optional:    true,
			},
		},
	}
}

// buildSSHExecutor builds a SSHExecutor (without connecting) from the target
// host and the ssh block configuration. The connection is established lazily on
// first use.
func buildSSHExecutor(target string, m *SSHModel, useSudo bool) (*exec.SSHExecutor, error) {
	if m == nil {
		m = &SSHModel{}
	}

	cfg, err := buildSSHClientConfig(m)
	if err != nil {
		return nil, err
	}

	jumps, err := buildSSHJumpChain(m, cfg)
	if err != nil {
		return nil, err
	}

	agentSocket, err := forwardAgentSocket(m)
	if err != nil {
		return nil, err
	}

	port := sshPort(m.Port, "ZEDAMIGO_SSH_PORT")
	addr := net.JoinHostPort(target, strconv.FormatInt(port, 10))

	return exec.NewSSH(exec.SSHParams{
		Addr:         addr,
		ClientConfig: cfg,
		Jumps:        jumps,
		UseSudo:      useSudo,
		AgentSocket:  agentSocket,
	}), nil
}

// forwardAgentSocket resolves the local SSH agent socket to forward to the
// target, or "" when ssh.forward_agent is off (the default). It fails closed:
// asking for forwarding with no agent running is a configuration error, and
// silently proceeding without it would surface much later as an unexplained
// authentication failure on the target.
//
// Note this is deliberately separate from use_agent (authenticating TO the
// target): the two are independent, and forwarding is useful even when the
// target is reached with a password or an explicit key file.
func forwardAgentSocket(m *SSHModel) (string, error) {
	if !sshBool(m.ForwardAgent, "ZEDAMIGO_SSH_FORWARD_AGENT") {
		return "", nil
	}

	sock := os.Getenv("SSH_AUTH_SOCK")
	if sock == "" {
		return "", fmt.Errorf("ssh.forward_agent is set but SSH_AUTH_SOCK is empty: " +
			"start an agent and load a key (eval \"$(ssh-agent)\" && ssh-add), or unset ssh.forward_agent")
	}

	return sock, nil
}

// buildSSHJumpChain parses the ssh.proxy_jump specification into an ordered
// chain of jump hosts. Each hop reuses the target's authentication methods and,
// unless overridden by a user@ prefix, its username; host key verification for
// jump hosts uses known_hosts or insecure_ignore_host_key. It returns nil when
// no proxy_jump is configured.
func buildSSHJumpChain(m *SSHModel, target *ssh.ClientConfig) ([]exec.JumpHost, error) {
	spec := sshStr(m.ProxyJump, "ZEDAMIGO_SSH_PROXY_JUMP")
	if spec == "" {
		return nil, nil
	}

	hostKeyCB, err := buildJumpHostKeyCallback(m)
	if err != nil {
		return nil, err
	}

	var hops []exec.JumpHost
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		user, host, port := parseJumpSpec(part)
		if user == "" {
			user = target.User
		}
		hops = append(hops, exec.JumpHost{
			Addr: net.JoinHostPort(host, port),
			ClientConfig: &ssh.ClientConfig{
				User:            user,
				Auth:            target.Auth,
				HostKeyCallback: hostKeyCB,
				Timeout:         target.Timeout,
			},
		})
	}
	if len(hops) == 0 {
		return nil, fmt.Errorf("ssh.proxy_jump %q did not specify any jump hosts", spec)
	}
	return hops, nil
}

// parseJumpSpec parses a single OpenSSH ProxyJump entry ([user@]host[:port]).
// A missing port defaults to 22; a missing user is returned empty so the caller
// can apply its default.
func parseJumpSpec(spec string) (user, host, port string) {
	hostport := spec
	if u, hp, found := strings.Cut(spec, "@"); found {
		user = u
		hostport = hp
	}
	host, port = splitHostPortDefault(hostport, "22")
	return user, host, port
}

// splitHostPortDefault splits host:port, defaulting the port when absent and
// stripping the brackets from a bare IPv6 literal.
func splitHostPortDefault(hostport, defPort string) (host, port string) {
	if h, p, err := net.SplitHostPort(hostport); err == nil {
		return h, p
	}
	h := hostport
	if strings.HasPrefix(h, "[") && strings.HasSuffix(h, "]") {
		h = h[1 : len(h)-1]
	}
	return h, defPort
}

func buildSSHClientConfig(m *SSHModel) (*ssh.ClientConfig, error) {
	var auths []ssh.AuthMethod

	if pw := sshStr(m.Password, "ZEDAMIGO_SSH_PASSWORD"); pw != "" {
		auths = append(auths, ssh.Password(pw))
	}

	keyData := []byte(sshStr(m.PrivateKey, "ZEDAMIGO_SSH_PRIVATE_KEY"))
	if len(keyData) == 0 {
		if f := sshStr(m.PrivateKeyFile, "ZEDAMIGO_SSH_PRIVATE_KEY_FILE"); f != "" {
			b, err := os.ReadFile(expandHome(f))
			if err != nil {
				return nil, fmt.Errorf("can't read SSH private key file %q: %w", f, err)
			}
			keyData = b
		}
	}
	if len(keyData) > 0 {
		var signer ssh.Signer
		var err error
		if pass := sshStr(m.PrivateKeyPassphrase, "ZEDAMIGO_SSH_PRIVATE_KEY_PASSPHRASE"); pass != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase(keyData, []byte(pass))
		} else {
			signer, err = ssh.ParsePrivateKey(keyData)
		}
		if err != nil {
			return nil, fmt.Errorf("invalid SSH private key: %w", err)
		}
		auths = append(auths, ssh.PublicKeys(signer))
	}

	if sshBool(m.UseAgent, "ZEDAMIGO_SSH_USE_AGENT") {
		sock := os.Getenv("SSH_AUTH_SOCK")
		if sock == "" {
			return nil, fmt.Errorf("ssh.use_agent is set but SSH_AUTH_SOCK is empty")
		}
		conn, err := net.Dial("unix", sock)
		if err != nil {
			return nil, fmt.Errorf("can't connect to SSH agent at %s: %w", sock, err)
		}
		// conn is intentionally kept open for the lifetime of the provider
		// process; the agent's Signers are invoked on each (re)connect.
		auths = append(auths, ssh.PublicKeysCallback(agent.NewClient(conn).Signers))
	}

	if len(auths) == 0 {
		return nil, fmt.Errorf("no SSH authentication method configured: set ssh.password, ssh.private_key / ssh.private_key_file, or ssh.use_agent (or the ZEDAMIGO_SSH_* env equivalents)")
	}

	hostKeyCB, err := buildHostKeyCallback(m)
	if err != nil {
		return nil, err
	}

	user := sshStr(m.User, "ZEDAMIGO_SSH_USER")
	if user == "" {
		if u, err := osuser.Current(); err == nil {
			user = u.Username
		}
	}

	return &ssh.ClientConfig{
		User:            user,
		Auth:            auths,
		HostKeyCallback: hostKeyCB,
		Timeout:         15 * time.Second,
	}, nil
}

// buildHostKeyCallback builds the SSH host key verification callback. It fails
// closed: if no verification method is configured and no default known_hosts
// file exists, it returns an error rather than silently trusting the host.
func buildHostKeyCallback(m *SSHModel) (ssh.HostKeyCallback, error) {
	if sshBool(m.InsecureIgnoreHostKey, "ZEDAMIGO_SSH_INSECURE") {
		return ssh.InsecureIgnoreHostKey(), nil //nolint:gosec // explicit opt-in for dev/test
	}

	if hk := sshStr(m.HostKey, "ZEDAMIGO_SSH_HOST_KEY"); hk != "" {
		pub, _, _, _, err := ssh.ParseAuthorizedKey([]byte(hk))
		if err != nil {
			return nil, fmt.Errorf("can't parse ssh.host_key: %w", err)
		}
		return ssh.FixedHostKey(pub), nil
	}

	cb, err := knownHostsCallback(m)
	if err != nil {
		return nil, err
	}
	if cb != nil {
		return cb, nil
	}

	return nil, fmt.Errorf("no SSH host key verification configured: set ssh.known_hosts_file or ssh.host_key, or (insecurely, for dev/test) ssh.insecure_ignore_host_key")
}

// buildJumpHostKeyCallback builds the host key verification callback used for
// jump hosts. Unlike the target, a single pinned ssh.host_key cannot verify the
// (potentially several, distinct) jump hosts, so only known_hosts or the
// insecure opt-out are accepted; it fails closed otherwise.
func buildJumpHostKeyCallback(m *SSHModel) (ssh.HostKeyCallback, error) {
	if sshBool(m.InsecureIgnoreHostKey, "ZEDAMIGO_SSH_INSECURE") {
		return ssh.InsecureIgnoreHostKey(), nil //nolint:gosec // explicit opt-in for dev/test
	}

	cb, err := knownHostsCallback(m)
	if err != nil {
		return nil, err
	}
	if cb != nil {
		return cb, nil
	}

	return nil, fmt.Errorf("ssh.proxy_jump requires host key verification for the jump host(s): set ssh.known_hosts_file, or (insecurely, for dev/test) ssh.insecure_ignore_host_key (ssh.host_key applies only to the final target)")
}

// knownHostsCallback returns a host key callback based on the configured
// known_hosts file, falling back to ~/.ssh/known_hosts when present. It returns
// (nil, nil) when no known_hosts file is configured or found.
func knownHostsCallback(m *SSHModel) (ssh.HostKeyCallback, error) {
	khFile := sshStr(m.KnownHostsFile, "ZEDAMIGO_SSH_KNOWN_HOSTS")
	if khFile == "" {
		if home, err := os.UserHomeDir(); err == nil {
			def := filepath.Join(home, ".ssh", "known_hosts")
			if _, err := os.Stat(def); err == nil {
				khFile = def
			}
		}
	}
	if khFile == "" {
		return nil, nil
	}
	cb, err := knownhosts.New(expandHome(khFile))
	if err != nil {
		return nil, fmt.Errorf("can't load known_hosts file %q: %w", khFile, err)
	}
	return cb, nil
}

// detectTargetPlatform returns the target's OS and architecture as GOOS/GOARCH
// values. For a local target these are the provider's own runtime values; for
// a remote target they are probed with `uname -sm` and normalized the same way
// install.sh does.
func detectTargetPlatform(ctx context.Context, ex exec.Executor, logDir string) (osName, arch string, err error) {
	if ex.IsLocal() {
		return runtime.GOOS, runtime.GOARCH, nil
	}

	res, err := ex.Run(ctx, logDir, "uname", "-sm")
	if err != nil {
		return "", "", fmt.Errorf("can't run `uname -sm` on the target: %w", err)
	}
	fields := strings.Fields(res.Stdout)
	if len(fields) != 2 {
		return "", "", fmt.Errorf("unexpected `uname -sm` output %q", strings.TrimSpace(res.Stdout))
	}

	switch fields[0] {
	case "Linux":
		osName = "linux"
	case "Darwin":
		osName = "darwin"
	default:
		return "", "", fmt.Errorf("unsupported target operating system %q", fields[0])
	}

	switch fields[1] {
	case "x86_64":
		arch = "amd64"
	case "aarch64", "arm64":
		arch = "arm64"
	default:
		return "", "", fmt.Errorf("unsupported target architecture %q", fields[1])
	}

	return osName, arch, nil
}

// resolveRemoteLibPath resolves the default lib_path on the remote host from
// its $HOME / $XDG_STATE_HOME, used when lib_path is not set explicitly.
func resolveRemoteLibPath(ctx context.Context, ex exec.Executor) (string, error) {
	logDir := "/tmp"
	res, err := ex.Run(ctx, logDir, "sh", "-c", `printf '%s\n' "${XDG_STATE_HOME:-$HOME/.local/state}"`)
	if err != nil {
		return "", fmt.Errorf("can't resolve remote state dir: %w", err)
	}
	base := strings.TrimSpace(res.Stdout)
	if base == "" {
		return "", fmt.Errorf("remote state dir resolved empty")
	}
	return path.Join(base, DefaultZedAmigoLibPath), nil
}

// bootstrapRemoteBinary ensures the provider binary is present on the remote
// host and returns its path. It runs the repo install script (pinned to the
// provider's version) in binary-only mode; the script auto-detects the remote
// architecture and prints the installed binary path as its final stdout line.
func bootstrapRemoteBinary(ctx context.Context, ex exec.Executor, libPath, version string) (string, error) {
	if version == "" || version == "dev" || version == "test" {
		return "", fmt.Errorf("cannot bootstrap the remote provider binary for version %q: set ssh.remote_binary_path to a pre-installed binary on the target", version)
	}

	v := strings.TrimPrefix(version, "v")
	url := fmt.Sprintf("https://raw.githubusercontent.com/andrei-zededa/terraform-provider-zedamigo/v%s/install.sh", v)
	script := fmt.Sprintf("curl -fsSL %s | bash -s -- --binary-only %s",
		shellSingleQuote(url), shellSingleQuote(v))

	logDir := path.Join(libPath, "bootstrap")
	res, err := ex.Run(ctx, logDir, "sh", "-c", script)
	if err != nil {
		return "", fmt.Errorf("remote binary bootstrap failed: %w; stderr: %s", err, res.Stderr)
	}

	// The script prints the absolute binary path as the final stdout line.
	binPath := lastNonEmptyLine(res.Stdout)
	if binPath == "" {
		return "", fmt.Errorf("remote binary bootstrap produced no path; stdout: %s; stderr: %s", res.Stdout, res.Stderr)
	}
	if _, err := ex.Stat(ctx, binPath); err != nil {
		return "", fmt.Errorf("bootstrapped remote binary %q not found: %w", binPath, err)
	}
	return binPath, nil
}

// --- small helpers ---

func sshStr(v types.String, env string) string {
	if !v.IsNull() && v.ValueString() != "" {
		return v.ValueString()
	}
	return os.Getenv(env)
}

func sshBool(v types.Bool, env string) bool {
	if !v.IsNull() {
		return v.ValueBool()
	}
	switch strings.ToLower(os.Getenv(env)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

func sshPort(v types.Int64, env string) int64 {
	if !v.IsNull() && v.ValueInt64() != 0 {
		return v.ValueInt64()
	}
	if s := os.Getenv(env); s != "" {
		if n, err := strconv.ParseInt(s, 10, 64); err == nil && n != 0 {
			return n
		}
	}
	return 22
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}

func lastNonEmptyLine(s string) string {
	lines := strings.Split(s, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if t := strings.TrimSpace(lines[i]); t != "" {
			return t
		}
	}
	return ""
}

// shellSingleQuote single-quotes a string for safe embedding in a /bin/sh
// command line.
func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
