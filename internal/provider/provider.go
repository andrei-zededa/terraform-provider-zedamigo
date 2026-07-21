// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/adrg/xdg"

	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/exec"
	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/hypervisor"
	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/undent"
)

const (
	// DefaultZedAmigoTarget is the default value of the provider target configuration
	// option.
	DefaultZedAmigoTarget = "localhost"
	// DefaultZedAmigoLibPath is the default value of the provider lib directory,
	// where all disk images and other files are created on `target`. This default
	// value is joined with `XDG_STATE_HOME`.
	DefaultZedAmigoLibPath = "zedamigo"
)

// Ensure ZedAmigoProvider satisfies various provider interfaces.
var (
	_ provider.Provider = &ZedAmigoProvider{}
)

// ZedAmigoProviderConfig encapsulates the provider configuration such as that
// it can be easily passed down to data sources and resources. It mostly tracks
// the exact paths of various commands(executables) that are needed by the various
// resources of the provider.
type ZedAmigoProviderConfig struct {
	Target      string
	LibPath     string
	UseSudo     bool
	Sudo        string
	QemuImg     string // Used by disk_image_resource
	Docker      string
	Swtpm       string
	Bash        string
	Flock       string // Used by host_reservation_resource (util-linux flock)
	GenISOImage string
	IP          string
	Hypervisor  hypervisor.Hypervisor

	// Exec is the executor used for ALL operations on `Target`: running
	// commands, filesystem access, process management and socket dialing.
	// It is a LocalExecutor when Target is localhost and an SSHExecutor
	// otherwise. Resources reach it as r.providerConf.Exec.
	Exec exec.Executor
}

// NewDefaultZedAmigoProviderConfig creates a new ZedAmigProviderConfig with
// default values.
func NewDefaultZedAmigoProviderConfig() ZedAmigoProviderConfig {
	return ZedAmigoProviderConfig{
		Target:  DefaultZedAmigoTarget,
		LibPath: filepath.Join(xdg.StateHome, DefaultZedAmigoLibPath),
	}
}

// ZedAmigoProvider defines the provider implementation.
type ZedAmigoProvider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running acceptance
	// testing.
	version string
}

// ZedAmigoProviderModel describes the provider data model.
type ZedAmigoProviderModel struct {
	Target  types.String `tfsdk:"target"`
	LibPath types.String `tfsdk:"lib_path"`
	UseSudo types.Bool   `tfsdk:"use_sudo"`
	SSH     *SSHModel    `tfsdk:"ssh"`
}

func (p *ZedAmigoProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "zedamigo"
	resp.Version = p.version
}

func (p *ZedAmigoProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"target": schema.StringAttribute{
				Description: "Target host on which to create resources, execute commands.",
				MarkdownDescription: undent.Md(`
				Target host on which the zedamigo provider will execute commands and
				create resources. Defaults to |localhost| (run everything locally). Set
				it to a hostname or IP address to operate on a remote host over SSH; in
				that case configure the connection in the |ssh| block. Optional.`),
				Optional: true,
			},
			"lib_path": schema.StringAttribute{
				Description: "Provider lib directory, where all files are created on `target`. Default: `XDG_STATE_HOME/zedamigo`.",
				MarkdownDescription: undent.Md(`
				The provider lib directory, where all disk images and other files are
				created on |target|. Optional and if not specified it defaults to
				|$XDG_STATE_HOME/zedamigo/|, e.g. |$HOME/.local/state/zedamigo/|. For a
				remote |target| the default is resolved from the remote host's
				environment.`),
				Optional: true,
			},
			"use_sudo": schema.BoolAttribute{
				Description: "Use `sudo` for running specific commands that need to be executed as the root user.",
				MarkdownDescription: undent.Md(`
				Use |sudo| for running specific (but not all) commands that need to
				be executed as the root user. Optional and if not specified it defaults
				to |false|.`),
				Optional: true,
			},
		},
		Blocks: map[string]schema.Block{
			"ssh": sshSchemaBlock(),
		},
	}
}

func (p *ZedAmigoProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var conf ZedAmigoProviderModel

	tflog.Info(ctx, "Configuring ZedAmigo")

	resp.Diagnostics.Append(req.Config.Get(ctx, &conf)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// If the user provided a configuration value for any of the attributes,
	// it must be a known value.

	// TODO: Most likely not needed for zedamigo but related to:
	// Checks for unknown configuration values. The method prevents an
	// unexpectedly misconfigured client, if Terraform configuration values
	// are only known after another resource is applied.
	if conf.Target.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("target"),
			"Unknown ZedAmigo target",
			"The provider cannot initialize as there is an unknown configuration value for the ZedAmigo target. "+
				"Either target apply the source of the value first, set the value statically in the configuration, or use the ZEDAMIGO_TARGET environment variable.",
		)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	// Default values to environment variables, but override with configuration
	// value if set.
	zaConf := NewDefaultZedAmigoProviderConfig()

	if t, exists := os.LookupEnv("ZEDAMIGO_TARGET"); exists {
		zaConf.Target = t
	}

	if !conf.Target.IsNull() {
		zaConf.Target = conf.Target.ValueString()
	}

	if zaConf.Target == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("target"),
			"ZedAmigo target cannot be empty.",
			"The provider cannot initialize as there is an empty configuration value for the ZedAmigo target. "+
				"Set a non-empty value in the configuration, or use the ZEDAMIGO_TARGET environment variable.",
		)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	ctx = tflog.SetField(ctx, "target", zaConf.Target)

	if !conf.LibPath.IsNull() {
		zaConf.LibPath = conf.LibPath.ValueString()
	}

	if zaConf.LibPath == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("lib_path"),
			"ZedAmigo lib_path cannot be empty.",
			"The provider cannot initialize as there is an empty configuration value for ZedAmigo lib_path. "+
				"Set a non-empty value in the configuration.",
		)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	// Determine whether privileged commands should be wrapped with sudo. This
	// is needed before building the executor.
	if !conf.UseSudo.IsNull() && conf.UseSudo.ValueBool() {
		zaConf.UseSudo = true
	}

	// Build the executor used for ALL operations on `Target`. For localhost a
	// LocalExecutor runs everything on the machine running the provider; for any
	// other target a SSHExecutor runs everything on the remote host. From here
	// on, filesystem and command operations go through zaConf.Exec.
	if zaConf.Target == DefaultZedAmigoTarget {
		zaConf.Exec = exec.NewLocal(zaConf.UseSudo)
	} else {
		sshExec, err := buildSSHExecutor(zaConf.Target, conf.SSH, zaConf.UseSudo)
		if err != nil {
			resp.Diagnostics.AddError("Invalid SSH configuration", err.Error())
			return
		}
		zaConf.Exec = sshExec

		// When lib_path was not set explicitly, resolve the default from the
		// remote host's environment rather than the local one.
		if conf.LibPath.IsNull() {
			rp, err := resolveRemoteLibPath(ctx, sshExec)
			if err != nil {
				resp.Diagnostics.AddError("Can't resolve remote lib_path",
					fmt.Sprintf("%v. Set lib_path explicitly in the provider configuration.", err))
				return
			}
			zaConf.LibPath = rp
			ctx = tflog.SetField(ctx, "lib_path", zaConf.LibPath)
		}

		// Resolve the provider binary path on the target (used by the
		// self-invoked daemons). Prefer an explicit remote_binary_path; otherwise
		// bootstrap it via the install script pinned to this provider's version.
		var rbp types.String
		if conf.SSH != nil {
			rbp = conf.SSH.RemoteBinaryPath
		}
		selfPath := sshStr(rbp, "ZEDAMIGO_REMOTE_BINARY_PATH")
		if selfPath == "" {
			selfPath, err = bootstrapRemoteBinary(ctx, sshExec, zaConf.LibPath, p.version)
			if err != nil {
				resp.Diagnostics.AddError("Can't provision the provider binary on the target", err.Error())
				return
			}
		}
		sshExec.SetSelfPath(selfPath)
	}

	if err := zaConf.Exec.MkdirAll(ctx, zaConf.LibPath, 0o700); err != nil {
		resp.Diagnostics.AddAttributeError(
			path.Root("lib_path"),
			fmt.Sprintf("%s", err),
			fmt.Sprintf("Failed to create ZedAmigo lib_path directory: %v", err),
		)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	ctx = tflog.SetField(ctx, "lib_path", zaConf.LibPath)

	if zaConf.UseSudo {
		sudo, err := zaConf.Exec.LookPath(ctx, "sudo")
		if err != nil {
			resp.Diagnostics.AddError("Can't find the `sudo` executable.",
				fmt.Sprintf("Can't find the `sudo` executable, got error: %v", err))
		}
		if resp.Diagnostics.HasError() {
			return
		}
		zaConf.Sudo = sudo
	}

	bash, err := zaConf.Exec.LookPath(ctx, "bash")
	if err != nil {
		resp.Diagnostics.AddError("Can't find bash.",
			fmt.Sprintf("Can't find bash, got error: %v", err))
	}
	if resp.Diagnostics.HasError() {
		return
	}
	zaConf.Bash = bash

	do, err := zaConf.Exec.LookPath(ctx, "docker")
	if err != nil {
		resp.Diagnostics.AddError("Can't find the `docker` executable.",
			fmt.Sprintf("Can't find the `docker` executable, got error: %v", err))
	}
	if resp.Diagnostics.HasError() {
		return
	}
	zaConf.Docker = do

	// Platform-specific tool lookups and hypervisor creation.
	configurePlatformTools(ctx, &zaConf, resp)
	if resp.Diagnostics.HasError() {
		return
	}

	// Make the provider config available during DataSource and Resource
	// type Configure methods.
	resp.DataSourceData = &zaConf
	resp.ResourceData = &zaConf

	tflog.Info(ctx, "Configured ZedAmigo", map[string]any{"success": true})
}

func (p *ZedAmigoProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewDiskImage,
		NewEveInstaller,
		NewInstalledNode,
		NewEdgeNode,
		NewVM,
		NewVirtualMachine,
		NewSwTPM,
		NewCloudInitISO,
		NewBridge,
		NewTAP,
		NewLAG,
		NewVLAN,
		NewDHCPServer,
		NewDHCP6Server,
		NewRADV,
		NewLocalDatastore,
		NewNetNS,
		NewInternetMonitor,
		NewMonitorSystemUsage,
		NewHostReservation,
	}
}

func (p *ZedAmigoProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewSystemInfoDataSource,
		NewEveInstallerDataSource,
	}
}

func newResourceID() (string, error) {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf("%08x", b), nil
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &ZedAmigoProvider{
			version: version,
		}
	}
}
