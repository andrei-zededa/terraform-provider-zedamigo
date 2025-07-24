// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/adrg/xdg"
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
// it can be easily passed down to data sources and resources.
type ZedAmigoProviderConfig struct {
	Target  string
	LibPath string
	Qemu    string
	QemuImg string
	Docker  string
	Swtpm   string
	Bash    string
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
}

func (p *ZedAmigoProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "zedamigo"
	resp.Version = p.version
}

func (p *ZedAmigoProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"target": schema.StringAttribute{
				Description:         "Target host on which to execute commands",
				MarkdownDescription: "Target host on which to execute commands",
				Optional:            true,
			},
			"lib_path": schema.StringAttribute{
				Description:         "The provider lib directory, where all disk images and other files are created on `target`. Default: `XDG_STATE_HOME/zedamigo`.",
				MarkdownDescription: "The provider lib directory, where all disk images and other files are created on `target`. Default: `XDG_STATE_HOME/zedamigo`.",
				Optional:            true,
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

	q, err := exec.LookPath("qemu-system-x86_64")
	if err != nil {
		resp.Diagnostics.AddError("Can't find the `qemu-system-x86_64` executable.",
			fmt.Sprintf("Can't find the `qemu-system-x86_64` executable, got error: %v", err))
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
		resp.Diagnostics.AddError("Can't find the `swtpm` executable.",
			fmt.Sprintf("Can't find the `swtpm` executable, got error: %v", err))
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
		NewSwTPM,
		NewCloudInitISO,
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
