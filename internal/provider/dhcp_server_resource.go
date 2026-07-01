// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"text/template"

	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/exec"
	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/undent"
	"github.com/davecgh/go-spew/spew"
	"github.com/hashicorp/terraform-plugin-framework-validators/objectvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

const (
	dhcpSrvsDir        = "dhcp_servers"
	dhcpConfigTemplate = `# CoreDHCP config for simple DHCP v4 server for a specific interface.
server4:
  listen:
    - "%{{ .Interface }}:67"
  plugins:
    - server_id: {{ .ServerID }}
    - dns: {{ .NameServer }}
    - router: {{ .Router }}
    - netmask: {{ .Netmask }}
    - range: {{ .LeasesFile }} {{ .PoolStart }} {{ .PoolEnd }} {{ .LeaseTime }}s
{{- range .StaticRoutes }}
    - staticroute: {{ .To.ValueString }},{{ .Via.ValueString }}
{{- end }}
`
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                = &DHCPServer{}
	_ resource.ResourceWithImportState = &DHCPServer{}
)

func NewDHCPServer() resource.Resource {
	return &DHCPServer{}
}

// DHCPServer defines the resource implementation.
type DHCPServer struct {
	providerConf *ZedAmigoProviderConfig
}

// DHCPPoolModel describes the DHCP pool configuration.
type DHCPPoolModel struct {
	Start types.String `tfsdk:"start"`
	End   types.String `tfsdk:"end"`
}

// DHCPStaticRouteModel describes a static route entry.
type DHCPStaticRouteModel struct {
	To  types.String `tfsdk:"to"`
	Via types.String `tfsdk:"via"`
}

// DHCPServerModel describes the resource data model.
type DHCPServerModel struct {
	ID           types.String           `tfsdk:"id"`
	Interface    types.String           `tfsdk:"interface"`
	ServerID     types.String           `tfsdk:"server_id"`
	NameServer   types.String           `tfsdk:"nameserver"`
	Router       types.String           `tfsdk:"router"`
	Netmask      types.String           `tfsdk:"netmask"`
	Pool         *DHCPPoolModel         `tfsdk:"pool"`
	LeaseTime    types.Int64            `tfsdk:"lease_time"`
	StaticRoutes []DHCPStaticRouteModel `tfsdk:"static_route"`
	LeasesFile   types.String           `tfsdk:"leases_file"`
	ConfigFile   types.String           `tfsdk:"config_file"`
	PIDFile      types.String           `tfsdk:"pid_file"`
	State        types.String           `tfsdk:"state"`
	NetNS        types.String           `tfsdk:"netns"`
}

func (r *DHCPServer) getResourceDir(id string) string {
	return filepath.Join(r.providerConf.LibPath, dhcpSrvsDir, id)
}

func (r *DHCPServer) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dhcp_server"
}

func (r *DHCPServer) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Simple DHCP v4 server for a specific interface",
		MarkdownDescription: undent.Md(`Create and manage a DHCP v4 server instance with a simple configuration
		that listens on a specific interface. Uses an embedded instance of CoreDHCP (https://github.com/coredhcp/coredhcp).
		NOTE: If the host has a firewall configuration that might drop incoming UDP port 67 packets. Double check that.
		This resource DOES NOT manage the host firewall configuration.`),

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				Description:         "DHCP server resource identifier",
				MarkdownDescription: "DHCP server resource identifier.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"interface": schema.StringAttribute{
				Description: "Interface on which to run this DHCP v4 server instance",
				Optional:    false,
				Required:    true,
			},
			"server_id": schema.StringAttribute{
				Description: "IPv4 address representing the DHCP server ID",
				Optional:    false,
				Required:    true,
			},
			"nameserver": schema.StringAttribute{
				Description: "Nameserver (DNS) IPv4 address",
				MarkdownDescription: undent.Md(`IPv4 address which will be used as the value for the nameserver/DNS option in the DHCP offer.
				If a fully working setup is needed then this must be an existing & working DNS resolver.
				This resource DOES NOT provide DNS resolving.`),
				Optional: false,
				Required: true,
			},
			"router": schema.StringAttribute{
				Description: "Router (gateway) IPv4 address",
				MarkdownDescription: undent.Md(`IPv4 address which will be used as the value for the router option in the DHCP offer.
				If a fully working setup is needed then the host must be configured to route (do NAT, etc.) correctly.
				This resource DOES NOT configure the host.`),
				Optional: false,
				Required: true,
			},
			"netmask": schema.StringAttribute{
				Description: "Netmask for the DHCP offers",
				Optional:    false,
				Required:    true,
			},
			"lease_time": schema.Int64Attribute{
				Description: "DHCP lease time in seconds",
				MarkdownDescription: undent.Md(`DHCP lease time in seconds. This determines how long a client can use an assigned IP address before needing to renew the lease.
				Defaults to 3600 seconds (1 hour).`),
				Optional: true,
				Computed: true,
			},
			"leases_file": schema.StringAttribute{
				Computed:    true,
				Description: "The sqlite3 leases file used by this instance of CoreDHCP",
			},
			"config_file": schema.StringAttribute{
				Computed:    true,
				Description: "The auto-generated CoreDHCP configuration file",
			},
			"pid_file": schema.StringAttribute{
				Computed:    true,
				Description: "Process ID file",
			},
			"state": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Desired state of the DHCP server daemon",
				MarkdownDescription: undent.Md(`Desired state of the DHCP server daemon. Can be "running" or "stopped".
				Defaults to "running". The provider will automatically start or stop the daemon to match this state.`),
			},
			"netns": schema.StringAttribute{
				Description: "Network namespace in which to run the DHCP server",
				Optional:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
		Blocks: map[string]schema.Block{
			"pool": schema.SingleNestedBlock{
				Description: "DHCP v4 address pool configuration for dynamic allocation",
				Validators: []validator.Object{
					objectvalidator.IsRequired(),
				},
				Attributes: map[string]schema.Attribute{
					"start": schema.StringAttribute{
						Description: "DHCP v4 pool first IPv4 address for dynamic allocation",
						Required:    true,
					},
					"end": schema.StringAttribute{
						Description: "DHCP v4 pool last IPv4 address for dynamic allocation",
						Required:    true,
					},
				},
			},
			"static_route": schema.ListNestedBlock{
				Description: "List of static routes to be advertised to DHCP clients.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"to": schema.StringAttribute{
							Description: "Destination CIDR for the static route (e.g. '192.168.2.0/24').",
							Required:    true,
						},
						"via": schema.StringAttribute{
							Description: "Gateway IP address for the static route.",
							Required:    true,
						},
					},
				},
			},
		},
	}
}

func (r *DHCPServer) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}

	conf, ok := req.ProviderData.(*ZedAmigoProviderConfig)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected string, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	r.providerConf = conf

	traceData := map[string]any{"providerConf": spew.Sprint(r.providerConf)}
	tflog.Trace(ctx, "DHCP server resource configure debugging", traceData)
}

func (r *DHCPServer) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data DHCPServerModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, err := newResourceID()
	if err != nil {
		resp.Diagnostics.AddError("DHCPServer Resource Error",
			fmt.Sprintf("Unable to generate a new resource ID: %s", err))
		return
	}
	data.ID = types.StringValue(id)
	if resp.Diagnostics.HasError() {
		return
	}

	d := r.getResourceDir(data.ID.ValueString())
	if err := r.providerConf.Exec.MkdirAll(ctx, d, 0o700); err != nil {
		resp.Diagnostics.AddError("DHCPServer Resource Error",
			fmt.Sprintf("Unable to create resource specific directory: %s", err))
		return
	}
	if err := createTFBackPointer(ctx, r.providerConf.Exec, d); err != nil {
		resp.Diagnostics.AddError("DHCPServer Resource Error",
			fmt.Sprintf("Unable to create resource specific file: %s", err))
		return
	}

	tmpl, err := template.New("config").Parse(dhcpConfigTemplate)
	if err != nil {
		resp.Diagnostics.AddError("DHCPServer Resource Error",
			fmt.Sprintf("Unable to parse config template: %s", err))
		return
	}

	leasesPath := filepath.Join(d, "leases.sqlite3")
	confPath := filepath.Join(d, "config.yaml")
	confFile, err := r.providerConf.Exec.OpenWrite(ctx, confPath, 0o644)
	if err != nil {
		resp.Diagnostics.AddError("DHCPServer Resource Error",
			fmt.Sprintf("Can't create file '%s': %s", confPath, err))
		return
	}
	defer confFile.Close()
	data.ConfigFile = types.StringValue(confPath)
	data.LeasesFile = types.StringValue(leasesPath)

	// Set default lease time if not specified.
	if data.LeaseTime.IsNull() || data.LeaseTime.IsUnknown() {
		data.LeaseTime = types.Int64Value(3600)
	}

	// Validate pool is not nil
	if data.Pool == nil {
		resp.Diagnostics.AddError("DHCPServer Resource Error",
			"Pool configuration is required but was not provided")
		return
	}

	// If we don't do this mapping and try to directly pass `data` to
	// template.Execute then that will call field.String() which returns
	// the value double-quoted.
	td := struct {
		Interface    string
		ServerID     string
		NameServer   string
		Router       string
		Netmask      string
		PoolStart    string
		PoolEnd      string
		LeaseTime    int64
		StaticRoutes []DHCPStaticRouteModel
		LeasesFile   string
	}{
		Interface:    data.Interface.ValueString(),
		ServerID:     data.ServerID.ValueString(),
		NameServer:   data.NameServer.ValueString(),
		Router:       data.Router.ValueString(),
		Netmask:      data.Netmask.ValueString(),
		PoolStart:    data.Pool.Start.ValueString(),
		PoolEnd:      data.Pool.End.ValueString(),
		LeaseTime:    data.LeaseTime.ValueInt64(),
		StaticRoutes: data.StaticRoutes,
		LeasesFile:   data.LeasesFile.ValueString(),
	}
	if err := tmpl.Execute(confFile, td); err != nil {
		resp.Diagnostics.AddError("DHCPServer Resource Error",
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
		if err := r.startDHCPServer(ctx, d, &data); err != nil {
			resp.Diagnostics.AddError("DHCPServer Resource Error",
				fmt.Sprintf("Failed to start DHCP server: %v", err))
			return
		}
	}

	// Read the DHCPServer current state.
	if diags, err := r.readDHCPServer(ctx, d, &data); err != nil {
		resp.Diagnostics.AddError("Failed to read DHCPServer state", err.Error())
		resp.Diagnostics.Append(diags...)
		return
	}

	tflog.Trace(ctx, "DHCPServer Resource created succesfully")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DHCPServer) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data DHCPServerModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d := r.getResourceDir(data.ID.ValueString())

	// Read the DHCPServer current state.
	if diags, err := r.readDHCPServer(ctx, d, &data); err != nil {
		if strings.Contains(err.Error(), "does not exist") {
			resp.State.RemoveResource(ctx)
			return
		}

		resp.Diagnostics.AddError("Failed to read DHCPServer state", err.Error())
		resp.Diagnostics.Append(diags...)
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DHCPServer) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan DHCPServerModel
	var state DHCPServerModel

	// Read Terraform plan and current state
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d := r.getResourceDir(state.ID.ValueString())

	// Validate pool is not nil
	if plan.Pool == nil || state.Pool == nil {
		resp.Diagnostics.AddError("DHCPServer Resource Update Error",
			"Pool configuration is required but was not provided")
		return
	}

	// Check if only the state field changed
	stateChanged := !plan.State.Equal(state.State)
	poolChanged := !plan.Pool.Start.Equal(state.Pool.Start) || !plan.Pool.End.Equal(state.Pool.End)
	configChanged := !plan.Interface.Equal(state.Interface) ||
		!plan.ServerID.Equal(state.ServerID) ||
		!plan.NameServer.Equal(state.NameServer) ||
		!plan.Router.Equal(state.Router) ||
		!plan.Netmask.Equal(state.Netmask) ||
		poolChanged ||
		!plan.LeaseTime.Equal(state.LeaseTime) ||
		!equalStaticRoutes(plan.StaticRoutes, state.StaticRoutes)

	if configChanged {
		resp.Diagnostics.AddError("DHCPServer Resource Update Error",
			"Configuration changes require resource recreation. Only the 'state' field can be updated in-place.")
		return
	}

	if stateChanged {
		desiredState := plan.State.ValueString()
		if desiredState == "" {
			desiredState = "running"
		}

		tflog.Info(ctx, "DHCP server state change requested", map[string]any{
			"from": state.State.ValueString(),
			"to":   desiredState,
		})

		if desiredState == "running" {
			if err := r.startDHCPServer(ctx, d, &plan); err != nil {
				resp.Diagnostics.AddError("DHCPServer Resource Update Error",
					fmt.Sprintf("Failed to start DHCP server: %v", err))
				return
			}
		} else if desiredState == "stopped" {
			if err := r.stopDHCPServer(ctx, d); err != nil {
				resp.Diagnostics.AddError("DHCPServer Resource Update Error",
					fmt.Sprintf("Failed to stop DHCP server: %v", err))
				return
			}
		}

		plan.State = types.StringValue(desiredState)
	}

	// Read back the current state to verify
	if diags, err := r.readDHCPServer(ctx, d, &plan); err != nil {
		resp.Diagnostics.AddError("Failed to read DHCPServer state after update", err.Error())
		resp.Diagnostics.Append(diags...)
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DHCPServer) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data DHCPServerModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d := r.getResourceDir(data.ID.ValueString())

	// Stop the DHCP server process if it's running.
	if err := r.stopDHCPServer(ctx, d); err != nil {
		// Log as warning instead of error, since the daemon might already be stopped
		tflog.Warn(ctx, "Failed to stop DHCP server during delete", map[string]any{"error": err.Error()})
	}

	if err := r.providerConf.Exec.Remove(ctx, d); err != nil {
		resp.Diagnostics.AddError("DHCPServer Resource Delete Error",
			fmt.Sprintf("Can't delete DHCPServer resource directory: %v", err))
		return
	}
}

func (r *DHCPServer) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// startDHCPServer starts the DHCP server daemon for the given resource.
// When netns is set, the daemon is started inside the network namespace using
// `ip netns exec <ns>`.
func (r *DHCPServer) startDHCPServer(ctx context.Context, d string, data *DHCPServerModel) error {
	netns := ""
	if !data.NetNS.IsNull() && !data.NetNS.IsUnknown() {
		netns = data.NetNS.ValueString()
	}

	self := r.providerConf.Exec.SelfPath()
	srvCmd := self
	srvArgs := []string{}
	if netns != "" {
		// Wrap with: ip netns exec <ns> <binary> ...
		// or: sudo -n ip netns exec <ns> <binary> ...
		if r.providerConf.UseSudo {
			srvCmd = r.providerConf.Sudo
			srvArgs = []string{"-n", r.providerConf.IP, "netns", "exec", netns, self}
		} else {
			srvCmd = r.providerConf.IP
			srvArgs = []string{"netns", "exec", netns, self}
		}
	} else if r.providerConf.UseSudo {
		srvCmd = r.providerConf.Sudo
		srvArgs = []string{"-n", self}
	}
	moreArgs := []string{"-pid-file", data.PIDFile.ValueString(), "-dhcp-server", "-ds.config", data.ConfigFile.ValueString()}
	if res, err := r.providerConf.Exec.RunDetached(ctx, d, srvCmd, append(srvArgs, moreArgs...)...); err != nil {
		return fmt.Errorf("failed to start DHCP server: %w, diagnostics: %v", err, res.Diagnostics())
	}
	return nil
}

// stopDHCPServer stops the DHCP server daemon for the given resource
func (r *DHCPServer) stopDHCPServer(ctx context.Context, d string) error {
	running, pid, err := readDHCPServerPID(ctx, r.providerConf.Exec, d)
	if err != nil {
		return fmt.Errorf("can't find DHCP server process: %w", err)
	}
	if !running {
		return nil // Already stopped
	}

	if err := r.providerConf.Exec.Kill(ctx, pid, syscall.SIGKILL); err != nil {
		return fmt.Errorf("can't kill DHCP server process: %w", err)
	}
	return nil
}

func (r *DHCPServer) readDHCPServer(ctx context.Context, resPath string, model *DHCPServerModel) (diag.Diagnostics, error) {
	// Check if resource directory exists
	if _, err := r.providerConf.Exec.Stat(ctx, resPath); exec.IsNotExist(err) {
		return nil, fmt.Errorf("resource directory does not exist")
	}

	// Determine desired state (default to "running" if not set)
	desiredState := "running"
	if !model.State.IsNull() && model.State.ValueString() != "" {
		desiredState = model.State.ValueString()
	}

	// Check if daemon is actually running
	running, _, _ := readDHCPServerPID(ctx, r.providerConf.Exec, resPath)
	actualState := "stopped"
	if running {
		actualState = "running"
	}

	// Self-healing: reconcile actual state with desired state
	if desiredState == "running" && actualState == "stopped" {
		// Daemon should be running but isn't - restart it
		tflog.Info(ctx, "DHCP server daemon is stopped but should be running, restarting...")
		if err := r.startDHCPServer(ctx, resPath, model); err != nil {
			return nil, fmt.Errorf("failed to restart DHCP server: %w", err)
		}
		actualState = "running"
	} else if desiredState == "stopped" && actualState == "running" {
		// Daemon should be stopped but is running - stop it
		tflog.Info(ctx, "DHCP server daemon is running but should be stopped, stopping...")
		if err := r.stopDHCPServer(ctx, resPath); err != nil {
			return nil, fmt.Errorf("failed to stop DHCP server: %w", err)
		}
		actualState = "stopped"
	}

	// Update state to match actual state
	model.State = types.StringValue(actualState)

	return nil, nil
}

func readDHCPServerPID(ctx context.Context, ex exec.Executor, path string) (bool, int, error) {
	pidPath := filepath.Join(path, "pid")
	x, err := ex.ReadFile(ctx, pidPath)
	if err != nil {
		return false, 0, fmt.Errorf("%w", err)
	}

	pid, err := strconv.ParseInt(string(bytes.TrimSpace(x)), 10, 32)
	if err != nil {
		return false, 0, fmt.Errorf("%w", err)
	}

	running, err := ex.IsRunning(ctx, int(pid), "")
	if err != nil {
		return false, int(pid), err
	}

	return running, int(pid), nil
}

func equalStaticRoutes(a, b []DHCPStaticRouteModel) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].To.Equal(b[i].To) || !a[i].Via.Equal(b[i].Via) {
			return false
		}
	}
	return true
}
