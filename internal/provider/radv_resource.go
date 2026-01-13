// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"

	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/cmd"
	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/undent"
	"github.com/davecgh/go-spew/spew"
	"github.com/gofrs/uuid/v5"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/shirou/gopsutil/v4/process"
)

const (
	radvDir            = "radv"
	radvConfigTemplate = `# Router Advertisement configuration
interface: {{ .Interface }}
prefix: {{ .Prefix }}
prefix_on_link: {{ .PrefixOnLink }}
prefix_autonomous: {{ .PrefixAutonomous }}
prefix_valid_lifetime: {{ .PrefixValidLifetime }}
prefix_preferred_lifetime: {{ .PrefixPreferredLifetime }}
routes: [{{ range $i, $e := .Routes }}{{if $i}},{{end}}"{{ .Prefix.ValueString }}"{{end}}]
dns_servers: {{ .DNSServers }}
managed_config: {{ .ManagedConfig }}
other_config: {{ .OtherConfig }}
router_lifetime: {{ .RouterLifetime }}
max_interval: {{ .MaxInterval }}
min_interval: {{ .MinInterval }}
hop_limit: {{ .HopLimit }}
`
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                = &RADV{}
	_ resource.ResourceWithImportState = &RADV{}
)

func NewRADV() resource.Resource {
	return &RADV{}
}

// RADV defines the resource implementation.
type RADV struct {
	providerConf *ZedAmigoProviderConfig
}

// RadvRouteModel describes a route entry.
type RadvRouteModel struct {
	Prefix types.String `tfsdk:"prefix"`
}

// RADVModel describes the resource data model.
type RADVModel struct {
	ID                      types.String     `tfsdk:"id"`
	Interface               types.String     `tfsdk:"interface"`
	Prefix                  types.String     `tfsdk:"prefix"`
	Routes                  []RadvRouteModel `tfsdk:"route"`
	PrefixOnLink            types.Bool       `tfsdk:"prefix_on_link"`
	PrefixAutonomous        types.Bool       `tfsdk:"prefix_autonomous"`
	PrefixValidLifetime     types.Int64      `tfsdk:"prefix_valid_lifetime"`
	PrefixPreferredLifetime types.Int64      `tfsdk:"prefix_preferred_lifetime"`
	DNSServers              types.String     `tfsdk:"dns_servers"`
	ManagedConfig           types.Bool       `tfsdk:"managed_config"`
	OtherConfig             types.Bool       `tfsdk:"other_config"`
	RouterLifetime          types.Int64      `tfsdk:"router_lifetime"`
	MaxInterval             types.Int64      `tfsdk:"max_interval"`
	MinInterval             types.Int64      `tfsdk:"min_interval"`
	HopLimit                types.Int64      `tfsdk:"hop_limit"`
	ConfigFile              types.String     `tfsdk:"config_file"`
	PIDFile                 types.String     `tfsdk:"pid_file"`
	State                   types.String     `tfsdk:"state"`
}

func (r *RADV) getResourceDir(id string) string {
	return filepath.Join(r.providerConf.LibPath, radvDir, id)
}

func (r *RADV) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_radv"
}

func (r *RADV) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "IPv6 Router Advertisement daemon",
		MarkdownDescription: undent.Md(`Create and manage an IPv6 Router Advertisement daemon for a specific interface.
		This resource sends periodic Router Advertisements (RAs) according to RFC 4861, enabling IPv6 autoconfiguration
		for clients on the network. Uses the github.com/mdlayher/ndp library.
		NOTE: This resource DOES NOT configure IP forwarding or firewall rules.`),

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				Description:         "RADV resource identifier",
				MarkdownDescription: "RADV resource identifier.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"interface": schema.StringAttribute{
				Description: "Interface on which to send Router Advertisements",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"prefix": schema.StringAttribute{
				Description: "IPv6 prefix in CIDR notation (e.g., 'fd00:abcd:1234::/64')",
				MarkdownDescription: undent.Md(`IPv6 prefix to advertise in Router Advertisements.
				This prefix will be used by clients for SLAAC (if prefix_autonomous is true).
				Example: 'fd00:abcd:1234::/64'`),
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"prefix_on_link": schema.BoolAttribute{
				Description: "Prefix on-link flag (L-bit)",
				MarkdownDescription: undent.Md(`When true, indicates that this prefix can be used for on-link determination.
				Typically set to true. Default: true`),
				Optional: true,
				Computed: true,
			},
			"prefix_autonomous": schema.BoolAttribute{
				Description: "Prefix autonomous flag (A-bit) for SLAAC",
				MarkdownDescription: undent.Md(`When true, indicates that this prefix can be used for stateless address
				autoconfiguration (SLAAC). Set to false if you want to use DHCPv6 only for addressing. Default: true`),
				Optional: true,
				Computed: true,
			},
			"prefix_valid_lifetime": schema.Int64Attribute{
				Description:         "Prefix valid lifetime in seconds",
				MarkdownDescription: "Length of time in seconds that the prefix is valid. Default: 86400 (24 hours)",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(86400),
			},
			"prefix_preferred_lifetime": schema.Int64Attribute{
				Description:         "Prefix preferred lifetime in seconds",
				MarkdownDescription: "Length of time in seconds that addresses generated from the prefix remain preferred. Default: 14400 (4 hours)",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(14400),
			},
			"dns_servers": schema.StringAttribute{
				Description: "Comma-separated list of DNS server IPv6 addresses",
				MarkdownDescription: undent.Md(`DNS server addresses to advertise via RDNSS option (RFC 8106).
				Multiple servers can be separated by commas. Example: '2606:4700:4700::1111,2606:4700:4700::1001'`),
				Optional: true,
			},
			"managed_config": schema.BoolAttribute{
				Description: "Managed address configuration flag (M-bit)",
				MarkdownDescription: undent.Md(`When true, tells clients to use DHCPv6 for address assignment.
				Set to true when using DHCPv6 for addresses instead of SLAAC. Default: false`),
				Optional: true,
				Computed: true,
			},
			"other_config": schema.BoolAttribute{
				Description: "Other configuration flag (O-bit)",
				MarkdownDescription: undent.Md(`When true, tells clients to use DHCPv6 for other configuration
				(e.g., DNS, NTP) beyond addresses. Default: false`),
				Optional: true,
				Computed: true,
			},
			"router_lifetime": schema.Int64Attribute{
				Description:         "Router lifetime in seconds",
				MarkdownDescription: "Lifetime associated with the default router in seconds. Default: 1800 (30 minutes)",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(1800),
			},
			"max_interval": schema.Int64Attribute{
				Description:         "Maximum time between unsolicited RAs in seconds",
				MarkdownDescription: "Maximum time allowed between sending unsolicited Router Advertisements. RFC 4861 recommends 600 seconds. Default: 600",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(600),
			},
			"min_interval": schema.Int64Attribute{
				Description:         "Minimum time between unsolicited RAs in seconds",
				MarkdownDescription: "Minimum time allowed between sending unsolicited Router Advertisements. RFC 4861 recommends at least 3 seconds and at most 0.75 * max_interval. Default: 200",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(200),
			},
			"hop_limit": schema.Int64Attribute{
				Description:         "Default hop limit for outgoing packets",
				MarkdownDescription: "The default value that should be placed in the Hop Count field of the IP header for outgoing packets. Default: 64",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(64),
			},
			"config_file": schema.StringAttribute{
				Computed:    true,
				Description: "The auto-generated RADV configuration file",
			},
			"pid_file": schema.StringAttribute{
				Computed:    true,
				Description: "Process ID file",
			},
			"state": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Desired state of the RADV daemon",
				MarkdownDescription: undent.Md(`Desired state of the RADV daemon. Can be "running" or "stopped".
				Defaults to "running". The provider will automatically start or stop the daemon to match this state.`),
			},
		},
		Blocks: map[string]schema.Block{
			"route": schema.ListNestedBlock{
				Description: "List of more-specific routes to be advertised to clients (RFC 4191).",
				Validators: []validator.List{
					listvalidator.SizeAtLeast(0),
				},
				PlanModifiers: []planmodifier.List{
					listplanmodifier.RequiresReplace(),
				},
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"prefix": schema.StringAttribute{
							Description: "Destination CIDR for the static route (e.g. '2001:db8:1::/48').",
							Required:    true,
						},
					},
				},
			},
		},
	}
}

func (r *RADV) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	conf, ok := req.ProviderData.(*ZedAmigoProviderConfig)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *ZedAmigoProviderConfig, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.providerConf = conf

	traceData := map[string]any{"providerConf": spew.Sprint(r.providerConf)}
	tflog.Trace(ctx, "RADV resource configure debugging", traceData)
}

func (r *RADV) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data RADVModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	u, err := uuid.NewV4()
	if err != nil {
		resp.Diagnostics.AddError("RADV Resource Error",
			fmt.Sprintf("Unable to generate a new UUID: %s", err))
		return
	}
	data.ID = types.StringValue(u.String())

	d := r.getResourceDir(data.ID.ValueString())
	if err := os.MkdirAll(d, 0o700); err != nil {
		resp.Diagnostics.AddError("RADV Resource Error",
			fmt.Sprintf("Unable to create resource specific directory: %s", err))
		return
	}
	if err := createTFBackPointer(d); err != nil {
		resp.Diagnostics.AddError("RADV Resource Error",
			fmt.Sprintf("Unable to create resource specific file: %s", err))
		return
	}

	// Set default values for boolean fields if not specified.
	if data.PrefixOnLink.IsNull() || data.PrefixOnLink.IsUnknown() {
		data.PrefixOnLink = types.BoolValue(true)
	}
	if data.PrefixAutonomous.IsNull() || data.PrefixAutonomous.IsUnknown() {
		data.PrefixAutonomous = types.BoolValue(true)
	}
	if data.ManagedConfig.IsNull() || data.ManagedConfig.IsUnknown() {
		data.ManagedConfig = types.BoolValue(false)
	}
	if data.OtherConfig.IsNull() || data.OtherConfig.IsUnknown() {
		data.OtherConfig = types.BoolValue(false)
	}

	tmpl, err := template.New("config").Parse(radvConfigTemplate)
	if err != nil {
		resp.Diagnostics.AddError("RADV Resource Error",
			fmt.Sprintf("Unable to parse config template: %s", err))
		return
	}

	confPath := filepath.Join(d, "config.yaml")
	confFile, err := os.Create(confPath)
	if err != nil {
		resp.Diagnostics.AddError("RADV Resource Error",
			fmt.Sprintf("Can't create file '%s': %s", confPath, err))
		return
	}
	defer confFile.Close()
	data.ConfigFile = types.StringValue(confPath)

	td := struct {
		Interface               string
		Prefix                  string
		Routes                  []RadvRouteModel
		PrefixOnLink            bool
		PrefixAutonomous        bool
		PrefixValidLifetime     int64
		PrefixPreferredLifetime int64
		DNSServers              string
		ManagedConfig           bool
		OtherConfig             bool
		RouterLifetime          int64
		MaxInterval             int64
		MinInterval             int64
		HopLimit                int64
	}{
		Interface:               data.Interface.ValueString(),
		Prefix:                  data.Prefix.ValueString(),
		Routes:                  data.Routes,
		PrefixOnLink:            data.PrefixOnLink.ValueBool(),
		PrefixAutonomous:        data.PrefixAutonomous.ValueBool(),
		PrefixValidLifetime:     data.PrefixValidLifetime.ValueInt64(),
		PrefixPreferredLifetime: data.PrefixPreferredLifetime.ValueInt64(),
		DNSServers:              data.DNSServers.ValueString(),
		ManagedConfig:           data.ManagedConfig.ValueBool(),
		OtherConfig:             data.OtherConfig.ValueBool(),
		RouterLifetime:          data.RouterLifetime.ValueInt64(),
		MaxInterval:             data.MaxInterval.ValueInt64(),
		MinInterval:             data.MinInterval.ValueInt64(),
		HopLimit:                data.HopLimit.ValueInt64(),
	}
	if err := tmpl.Execute(confFile, td); err != nil {
		resp.Diagnostics.AddError("RADV Resource Error",
			fmt.Sprintf("Can't write templated config file '%s': %s", confPath, err))
		return
	}

	pidFile := filepath.Join(d, "pid")
	data.PIDFile = types.StringValue(pidFile)

	// Set default state to "running" if not specified
	if data.State.IsNull() || data.State.ValueString() == "" {
		data.State = types.StringValue("running")
	}

	// Only start the daemon if state is "running"
	if data.State.ValueString() == "running" {
		if err := r.startRADV(d, &data); err != nil {
			resp.Diagnostics.AddError("RADV Resource Error",
				fmt.Sprintf("Failed to start RADV daemon: %v", err))
			return
		}
	}

	// Read the RADV current state
	if diags, err := r.readRADV(d, &data); err != nil {
		resp.Diagnostics.AddError("Failed to read RADV state", err.Error())
		resp.Diagnostics.Append(diags...)
		return
	}

	tflog.Trace(ctx, "RADV Resource created successfully")

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RADV) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data RADVModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d := r.getResourceDir(data.ID.ValueString())

	if diags, err := r.readRADV(d, &data); err != nil {
		if strings.Contains(err.Error(), "does not exist") {
			resp.State.RemoveResource(ctx)
			return
		}

		resp.Diagnostics.AddError("Failed to read RADV state", err.Error())
		resp.Diagnostics.Append(diags...)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RADV) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan RADVModel
	var state RADVModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d := r.getResourceDir(state.ID.ValueString())

	// Check if only the state field changed
	stateChanged := !plan.State.Equal(state.State)

	// Most config changes require recreation (marked with RequiresReplace)
	// Only state changes are allowed in-place

	if stateChanged {
		desiredState := plan.State.ValueString()
		if desiredState == "" {
			desiredState = "running"
		}

		tflog.Info(ctx, "RADV state change requested", map[string]any{
			"from": state.State.ValueString(),
			"to":   desiredState,
		})

		if desiredState == "running" {
			if err := r.startRADV(d, &plan); err != nil {
				resp.Diagnostics.AddError("RADV Resource Update Error",
					fmt.Sprintf("Failed to start RADV daemon: %v", err))
				return
			}
		} else if desiredState == "stopped" {
			if err := r.stopRADV(d); err != nil {
				resp.Diagnostics.AddError("RADV Resource Update Error",
					fmt.Sprintf("Failed to stop RADV daemon: %v", err))
				return
			}
		}

		plan.State = types.StringValue(desiredState)
	}

	if diags, err := r.readRADV(d, &plan); err != nil {
		resp.Diagnostics.AddError("Failed to read RADV state after update", err.Error())
		resp.Diagnostics.Append(diags...)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *RADV) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data RADVModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d := r.getResourceDir(data.ID.ValueString())

	if err := r.stopRADV(d); err != nil {
		tflog.Warn(ctx, "Failed to stop RADV daemon during delete", map[string]any{"error": err.Error()})
	}

	if err := os.RemoveAll(d); err != nil {
		resp.Diagnostics.AddError("RADV Resource Delete Error",
			fmt.Sprintf("Can't delete RADV resource directory: %v", err))
		return
	}
}

func (r *RADV) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *RADV) startRADV(d string, data *RADVModel) error {
	srvCmd := os.Args[0]
	srvArgs := []string{}
	if r.providerConf.UseSudo {
		srvCmd = r.providerConf.Sudo
		srvArgs = []string{"-n", os.Args[0]}
	}
	moreArgs := []string{"-pid-file", data.PIDFile.ValueString(), "-radv", "-radv.wait", "-radv.config", data.ConfigFile.ValueString()}
	if res, err := cmd.RunDetached(d, srvCmd, append(srvArgs, moreArgs...)...); err != nil {
		return fmt.Errorf("failed to start RADV daemon: %w, diagnostics: %v", err, res.Diagnostics())
	}
	return nil
}

func (r *RADV) stopRADV(d string) error {
	running, proc, err := readRADVPID(d)
	if err != nil {
		return fmt.Errorf("can't find RADV daemon process: %w", err)
	}
	if !running {
		return nil
	}

	var killErr error
	if r.providerConf.UseSudo {
		killCmd := r.providerConf.Sudo
		killArgs := []string{"-n", "kill", fmt.Sprintf("%d", proc.Pid)}
		res, err := cmd.Run(d, killCmd, killArgs...)
		if err != nil {
			killErr = err
		} else if res.ExitCode != 0 {
			killErr = fmt.Errorf("sudo kill failed with exit code %d: %s", res.ExitCode, res.Stderr)
		}
	} else {
		killErr = proc.Kill()
	}

	if killErr != nil {
		return fmt.Errorf("can't kill RADV daemon process: %w", killErr)
	}
	return nil
}

func (r *RADV) readRADV(resPath string, model *RADVModel) (diag.Diagnostics, error) {
	if _, err := os.Stat(resPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("resource directory does not exist")
	}

	desiredState := "running"
	if !model.State.IsNull() && model.State.ValueString() != "" {
		desiredState = model.State.ValueString()
	}

	running, _, _ := readRADVPID(resPath)
	actualState := "stopped"
	if running {
		actualState = "running"
	}

	// Self-healing: reconcile actual state with desired state.
	if desiredState == "running" && actualState == "stopped" {
		tflog.Info(context.Background(), "RADV daemon is stopped but should be running, restarting...")
		if err := r.startRADV(resPath, model); err != nil {
			return nil, fmt.Errorf("failed to restart RADV daemon: %w", err)
		}
		actualState = "running"
	} else if desiredState == "stopped" && actualState == "running" {
		tflog.Info(context.Background(), "RADV daemon is running but should be stopped, stopping...")
		if err := r.stopRADV(resPath); err != nil {
			return nil, fmt.Errorf("failed to stop RADV daemon: %w", err)
		}
		actualState = "stopped"
	}

	model.State = types.StringValue(actualState)

	return nil, nil
}

func readRADVPID(path string) (bool, *process.Process, error) {
	pidPath := filepath.Join(path, "pid")
	x, err := os.ReadFile(pidPath)
	if err != nil {
		return false, nil, fmt.Errorf("%w", err)
	}

	pid, err := strconv.ParseInt(string(bytes.TrimSpace(x)), 10, 32)
	if err != nil {
		return false, nil, fmt.Errorf("%w", err)
	}

	p, err := process.NewProcess(int32(pid))
	if err != nil {
		return false, nil, fmt.Errorf("%w", err)
	}

	running, err := p.IsRunning()
	if err != nil || !running {
		return false, p, fmt.Errorf("process %d is not running", pid)
	}

	return true, p, nil
}
