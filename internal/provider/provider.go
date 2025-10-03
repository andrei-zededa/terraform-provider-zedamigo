// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/adrg/xdg"

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
	Target       string
	LibPath      string
	BaseOVMFCode string
	BaseOVMFVars string
	UseSudo      bool
	Sudo         string
	Qemu         string
	QemuImg      string
	Docker       string
	Swtpm        string
	Bash         string
	GenISOImage  string
	IP           string
}

// NewDefaultZedAmigoProviderConfig creates a new ZedAmigProviderConfig with
// default values.
func NewDefaultZedAmigoProviderConfig() ZedAmigoProviderConfig {
	return ZedAmigoProviderConfig{
		Target:  DefaultZedAmigoTarget,
		LibPath: filepath.Join(xdg.CacheHome, DefaultZedAmigoLibPath),
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
				create resources. ONLY |localhost| is currently supported. Optional and
				if not specified it defaults to |localhost|.`),
				Optional: true,
			},
			"lib_path": schema.StringAttribute{
				Description: "Provider lib directory, where all files are created on `target`. Default: `XDG_STATE_HOME/zedamigo`.",
				MarkdownDescription: undent.Md(`
				The provider lib directory, where all disk images and other files are
				created on |target|. Optional and if not specified it defaults to
				|$XDG_STATE_HOME/zedamigo/|, e.g. |$HOME/.local/state/zedamigo/|.`),
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

	if err := os.MkdirAll(zaConf.LibPath, 0o700); err != nil {
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

	if err := os.MkdirAll(filepath.Join(zaConf.LibPath, embeddedOVMFTargetDir), 0o700); err != nil {
		resp.Diagnostics.AddAttributeError(
			path.Root("lib_path"),
			fmt.Sprintf("%s", err),
			fmt.Sprintf("Failed to create ZedAmigo lib_path/embedded_ovmf directory: %v", err),
		)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	for _, f := range embeddedOVMFFiles {
		tFile := filepath.Base(f)
		tPath := filepath.Join(zaConf.LibPath, embeddedOVMFTargetDir, tFile)
		if err := extractFileIfNotExists(f, tPath); err != nil {
			resp.Diagnostics.AddAttributeError(
				path.Root("lib_path"),
				fmt.Sprintf("%s", err),
				fmt.Sprintf("Failed to extract OVMF file '%s': %v", f, err),
			)
		}
		if strings.Contains(tFile, "CODE") || strings.Contains(tFile, "code") {
			zaConf.BaseOVMFCode = tPath
		}
		if strings.Contains(tFile, "VARS") || strings.Contains(tFile, "vars") {
			zaConf.BaseOVMFVars = tPath
		}
	}
	if resp.Diagnostics.HasError() {
		return
	}

	if !conf.UseSudo.IsNull() && conf.UseSudo.ValueBool() {
		zaConf.UseSudo = true
		sudo, err := exec.LookPath("sudo")
		if err != nil {
			resp.Diagnostics.AddError("Can't find the `sudo` executable.",
				fmt.Sprintf("Can't find the `sudo` executable, got error: %v", err))
		}
		if resp.Diagnostics.HasError() {
			return
		}
		zaConf.Sudo = sudo

		// TODO: Might want to add here a symlink resolv step.
	}

	q, err := exec.LookPath(qemuSystemCmd)
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Can't find the `%s` executable.", qemuSystemCmd),
			fmt.Sprintf("Can't find the `%s` executable, got error: %v", qemuSystemCmd, err))
	}
	if resp.Diagnostics.HasError() {
		return
	}
	zaConf.Qemu = q

	qi, err := exec.LookPath("qemu-img")
	if err != nil {
		resp.Diagnostics.AddError("Can't find the `qemu-img` executable.",
			fmt.Sprintf("Can't find the `qemu-img` executable, got error: %v", err))
	}
	if resp.Diagnostics.HasError() {
		return
	}
	zaConf.QemuImg = qi

	do, err := exec.LookPath("docker")
	if err != nil {
		resp.Diagnostics.AddError("Can't find the `docker` executable.",
			fmt.Sprintf("Can't find the `docker` executable, got error: %v", err))
	}
	if resp.Diagnostics.HasError() {
		return
	}
	zaConf.Docker = do

	st, err := exec.LookPath("swtpm")
	if err != nil {
		resp.Diagnostics.AddWarning("Can't find the `swtpm` executable.",
			fmt.Sprintf("This warning can be ignored if you DO NOT use the SwTPM resource. Can't find `swtpm`, got error: %v", err))
	}
	if resp.Diagnostics.HasError() {
		return
	}
	zaConf.Swtpm = st
	if stAbs, err := filepath.Abs(st); err != nil {
		tflog.Debug(ctx, "filepath.Abs error", map[string]any{"error": err})
	} else {
		zaConf.Swtpm = stAbs
		if stReal, err := filepath.EvalSymlinks(stAbs); err != nil {
			tflog.Debug(ctx, "filepath.EvalSymlinks error", map[string]any{"error": err})
		} else {
			zaConf.Swtpm = stReal
		}
	}

	bash, err := exec.LookPath("bash")
	if err != nil {
		resp.Diagnostics.AddError("Can't find bash.",
			fmt.Sprintf("Can't find bash, got error: %v", err))
	}
	if resp.Diagnostics.HasError() {
		return
	}
	zaConf.Bash = bash

	ip, err := exec.LookPath("ip")
	if err != nil {
		resp.Diagnostics.AddWarning("Can't find `ip`. Any resources that depend on it like bridge, tap, vlan will NOT work.",
			fmt.Sprintf("This warning can be ignored if you don't use bridge, tap or vlan resources. Can't find `ip`, got error: %v", err))
	}
	if resp.Diagnostics.HasError() {
		return
	}
	zaConf.IP = ip

	gencmd, err := exec.LookPath("genisoimage")
	if err != nil {
		resp.Diagnostics.AddWarning("Can't find the `genisoimage` executable (part of the `cdrkit` package or the `genisoimage` package).",
			fmt.Sprintf("This warning can be ignored if you DO NOT use the Cloud Init ISO resource. Can't find `genisoimage`, got error: %v", err))
	}
	if resp.Diagnostics.HasError() {
		return
	}
	zaConf.GenISOImage = gencmd

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
		NewVLAN,
		NewDHCPServer,
	}
}

func (p *ZedAmigoProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewSystemInfoDataSource,
		NewEveInstallerDataSource,
	}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &ZedAmigoProvider{
			version: version,
		}
	}
}

// extractFileIfNotExists checks if a file exists at targetPath, and if not,
// extracts it from the embedded filesystem.
func extractFileIfNotExists(embeddedPath, targetPath string) error {
	// Check if file already exists
	if _, err := os.Stat(targetPath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("error checking if file exists: %w", err)
	}

	// File doesn't exist, so extract it.
	// Read the embedded file.
	data, err := embeddedOVMF.ReadFile(embeddedPath)
	if err != nil {
		return fmt.Errorf("error reading embedded file %s: %w", embeddedPath, err)
	}

	// Write the file to the target path.
	if err := os.WriteFile(targetPath, data, 0o644); err != nil {
		return fmt.Errorf("error writing file to %s: %w", targetPath, err)
	}

	return nil
}

// shortID returns a 7-char Base58 string, uniformly random over 58^7.
func shortID() string {
	const (
		alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
		length   = 7
		base     = uint64(58)
		space    = uint64(2207984167552) // 58^7
	)

	n := rand.Uint64N(space) // unbiased in [0, 58^7)

	var buf [length]byte
	for i := length - 1; i >= 0; i-- {
		buf[i] = alphabet[int(n%base)]
		n /= base
	}
	return string(buf[:])
}
