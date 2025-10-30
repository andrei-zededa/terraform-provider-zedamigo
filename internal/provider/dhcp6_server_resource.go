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
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/shirou/gopsutil/v4/process"
)

const (
	dhcp6SrvsDir        = "dhcp6_servers"
	dhcp6ConfigTemplate = `# CoreDHCP config for simple DHCP v6 server for a specific interface.
server6:
  listen:
    - "[ff02::1:2%{{ .Interface }}]"
  plugins:
    # server_id is mandatory for RFC-compliant operation.
    # - server_id: <DUID format> <LL address>
    # The supported DUID formats are LL and LLT
    - server_id: LL {{ .ServerID }}
    - dns: {{ .NameServer }}
    - range: {{ .LeasesFile }} {{ .PoolStart }} {{ .PoolEnd }} {{ .LeaseTime }}s
{{ if .Prefix }}    - prefix: "{{ .Prefix }}"{{ end }}
`
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                = &DHCP6Server{}
	_ resource.ResourceWithImportState = &DHCP6Server{}
)

func NewDHCP6Server() resource.Resource {
	return &DHCP6Server{}
}

// DHCP6Server defines the resource implementation.
type DHCP6Server struct {
	providerConf *ZedAmigoProviderConfig
}

// DHCP6PoolModel describes the DHCPv6 pool configuration.
type DHCP6PoolModel struct {
	Start types.String `tfsdk:"start"`
	End   types.String `tfsdk:"end"`
}

// DHCP6ServerModel describes the resource data model.
type DHCP6ServerModel struct {
	ID         types.String     `tfsdk:"id"`
	Interface  types.String     `tfsdk:"interface"`
	ServerID   types.String     `tfsdk:"server_id"`
	Prefix     types.String     `tfsdk:"prefix"`
	NameServer types.String     `tfsdk:"nameserver"`
	Pool       *DHCP6PoolModel  `tfsdk:"pool"`
	LeaseTime  types.Int64      `tfsdk:"lease_time"`
	LeasesFile types.String     `tfsdk:"leases_file"`
	ConfigFile types.String     `tfsdk:"config_file"`
	PIDFile    types.String     `tfsdk:"pid_file"`
	State      types.String     `tfsdk:"state"`
}

func (r *DHCP6Server) getResourceDir(id string) string {
	return filepath.Join(r.providerConf.LibPath, dhcp6SrvsDir, id)
}

func (r *DHCP6Server) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dhcp6_server"
}

func (r *DHCP6Server) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Simple DHCP v6 server for a specific interface",
		MarkdownDescription: undent.Md(`Create and manage a DHCP v6 server instance with a simple configuration
		that listens on a specific interface. Uses an embedded instance of CoreDHCP (https://github.com/coredhcp/coredhcp).
		NOTE: If the host has a firewall configuration that might drop incoming UDP port 547 packets. Double check that.
		This resource DOES NOT manage the host firewall configuration.`),

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				Description:         "DHCPv6 server resource identifier",
				MarkdownDescription: "DHCPv6 server resource identifier.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"interface": schema.StringAttribute{
				Description: "Interface on which to run this DHCP v6 server instance",
				Optional:    false,
				Required:    true,
			},
			"server_id": schema.StringAttribute{
				Description: "MAC address representing the DHCPv6 server ID",
				Optional:    false,
				Required:    true,
			},
			"prefix": schema.StringAttribute{
				Description:         "Prefix delegation",
				MarkdownDescription: undent.Md(`Prefix delegation`),
				Optional:            true,
				Required:            false,
			},
			"nameserver": schema.StringAttribute{
				Description: "Nameserver (DNS) IPv6 address",
				MarkdownDescription: undent.Md(`IPv6 address which will be used as the value for the nameserver/DNS option in the DHCP offer.
				If a fully working setup is needed then this must be an existing & working DNS resolver.
				This resource DOES NOT provide DNS resolving.`),
				Optional: false,
				Required: true,
			},
			"pool": schema.SingleNestedAttribute{
				Description: "DHCP v6 address pool configuration for dynamic allocation",
				Required:    true,
				Attributes: map[string]schema.Attribute{
					"start": schema.StringAttribute{
						Description: "DHCP v6 pool first IPv6 address for dynamic allocation",
						Required:    true,
					},
					"end": schema.StringAttribute{
						Description: "DHCP v6 pool last IPv6 address for dynamic allocation",
						Required:    true,
					},
				},
			},
			"lease_time": schema.Int64Attribute{
				Description: "DHCPv6 lease time in seconds",
				MarkdownDescription: undent.Md(`DHCPv6 lease time in seconds. This determines how long a client can use an assigned IPv6 address before needing to renew the lease.
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
				Description: "Desired state of the DHCPv6 server daemon",
				MarkdownDescription: undent.Md(`Desired state of the DHCPv6 server daemon. Can be "running" or "stopped".
				Defaults to "running". The provider will automatically start or stop the daemon to match this state.`),
			},
		},
	}
}

func (r *DHCP6Server) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
	tflog.Trace(ctx, "DHCPv6 server resource configure debugging", traceData)
}

func (r *DHCP6Server) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data DHCP6ServerModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	u, err := uuid.NewV4()
	if err != nil {
		resp.Diagnostics.AddError("DHCP6Server Resource Error",
			fmt.Sprintf("Unable to generate a new UUID: %s", err))
		return
	}
	data.ID = types.StringValue(u.String())
	if resp.Diagnostics.HasError() {
		return
	}

	d := r.getResourceDir(data.ID.ValueString())
	if err := os.MkdirAll(d, 0o700); err != nil {
		resp.Diagnostics.AddError("DHCP6Server Resource Error",
			fmt.Sprintf("Unable to create resource specific directory: %s", err))
		return
	}
	if err := createTFBackPointer(d); err != nil {
		resp.Diagnostics.AddError("DHCP6Server Resource Error",
			fmt.Sprintf("Unable to create resource specific file: %s", err))
		return
	}

	tmpl, err := template.New("config").Parse(dhcp6ConfigTemplate)
	if err != nil {
		resp.Diagnostics.AddError("DHCP6Server Resource Error",
			fmt.Sprintf("Unable to parse config template: %s", err))
		return
	}

	leasesPath := filepath.Join(d, "leases.sqlite3")
	confPath := filepath.Join(d, "config.yaml")
	confFile, err := os.Create(confPath)
	if err != nil {
		resp.Diagnostics.AddError("DHCP6Server Resource Error",
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
		resp.Diagnostics.AddError("DHCP6Server Resource Error",
			"Pool configuration is required but was not provided")
		return
	}

	// If we don't do this mapping and try to directly pass `data` to
	// template.Execute then that will call field.String() which returns
	// the value double-quoted.
	td := struct {
		Interface  string
		ServerID   string
		Prefix     string
		NameServer string
		PoolStart  string
		PoolEnd    string
		LeaseTime  int64
		LeasesFile string
	}{
		Interface:  data.Interface.ValueString(),
		ServerID:   data.ServerID.ValueString(),
		Prefix:     data.Prefix.ValueString(),
		NameServer: data.NameServer.ValueString(),
		PoolStart:  data.Pool.Start.ValueString(),
		PoolEnd:    data.Pool.End.ValueString(),
		LeaseTime:  data.LeaseTime.ValueInt64(),
		LeasesFile: data.LeasesFile.ValueString(),
	}
	if err := tmpl.Execute(confFile, td); err != nil {
		resp.Diagnostics.AddError("DHCP6Server Resource Error",
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
		if err := r.startDHCP6Server(d, &data); err != nil {
			resp.Diagnostics.AddError("DHCP6Server Resource Error",
				fmt.Sprintf("Failed to start DHCPv6 server: %v", err))
			return
		}
	}

	// Read the DHCP6Server current state.
	if diags, err := r.readDHCP6Server(d, &data); err != nil {
		resp.Diagnostics.AddError("Failed to read DHCP6Server state", err.Error())
		resp.Diagnostics.Append(diags...)
		return
	}

	tflog.Trace(ctx, "DHCP6Server Resource created succesfully")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DHCP6Server) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data DHCP6ServerModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d := r.getResourceDir(data.ID.ValueString())

	// Read the DHCP6Server current state.
	if diags, err := r.readDHCP6Server(d, &data); err != nil {
		if strings.Contains(err.Error(), "does not exist") {
			resp.State.RemoveResource(ctx)
			return
		}

		resp.Diagnostics.AddError("Failed to read DHCP6Server state", err.Error())
		resp.Diagnostics.Append(diags...)
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DHCP6Server) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan DHCP6ServerModel
	var state DHCP6ServerModel

	// Read Terraform plan and current state
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d := r.getResourceDir(state.ID.ValueString())

	// Validate pool is not nil
	if plan.Pool == nil || state.Pool == nil {
		resp.Diagnostics.AddError("DHCP6Server Resource Update Error",
			"Pool configuration is required but was not provided")
		return
	}

	// Check if only the state field changed
	stateChanged := !plan.State.Equal(state.State)
	poolChanged := !plan.Pool.Start.Equal(state.Pool.Start) || !plan.Pool.End.Equal(state.Pool.End)
	configChanged := !plan.Interface.Equal(state.Interface) ||
		!plan.ServerID.Equal(state.ServerID) ||
		!plan.Prefix.Equal(state.Prefix) ||
		!plan.NameServer.Equal(state.NameServer) ||
		poolChanged ||
		!plan.LeaseTime.Equal(state.LeaseTime)

	if configChanged {
		resp.Diagnostics.AddError("DHCP6Server Resource Update Error",
			"Configuration changes require resource recreation. Only the 'state' field can be updated in-place.")
		return
	}

	if stateChanged {
		desiredState := plan.State.ValueString()
		if desiredState == "" {
			desiredState = "running"
		}

		tflog.Info(ctx, "DHCPv6 server state change requested", map[string]any{
			"from": state.State.ValueString(),
			"to":   desiredState,
		})

		if desiredState == "running" {
			if err := r.startDHCP6Server(d, &plan); err != nil {
				resp.Diagnostics.AddError("DHCP6Server Resource Update Error",
					fmt.Sprintf("Failed to start DHCPv6 server: %v", err))
				return
			}
		} else if desiredState == "stopped" {
			if err := r.stopDHCP6Server(d); err != nil {
				resp.Diagnostics.AddError("DHCP6Server Resource Update Error",
					fmt.Sprintf("Failed to stop DHCPv6 server: %v", err))
				return
			}
		}

		plan.State = types.StringValue(desiredState)
	}

	// Read back the current state to verify
	if diags, err := r.readDHCP6Server(d, &plan); err != nil {
		resp.Diagnostics.AddError("Failed to read DHCP6Server state after update", err.Error())
		resp.Diagnostics.Append(diags...)
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DHCP6Server) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data DHCP6ServerModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d := r.getResourceDir(data.ID.ValueString())

	// Stop the DHCPv6 server process if it's running.
	if err := r.stopDHCP6Server(d); err != nil {
		// Log as warning instead of error, since the daemon might already be stopped
		tflog.Warn(ctx, "Failed to stop DHCPv6 server during delete", map[string]any{"error": err.Error()})
	}

	if err := os.RemoveAll(d); err != nil {
		resp.Diagnostics.AddError("DHCP6Server Resource Delete Error",
			fmt.Sprintf("Can't delete DHCP6Server resource directory: %v", err))
		return
	}
}

func (r *DHCP6Server) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// startDHCP6Server starts the DHCPv6 server daemon for the given resource
func (r *DHCP6Server) startDHCP6Server(d string, data *DHCP6ServerModel) error {
	srvCmd := os.Args[0]
	srvArgs := []string{}
	if r.providerConf.UseSudo {
		srvCmd = r.providerConf.Sudo
		srvArgs = []string{"-n", os.Args[0]}
	}
	moreArgs := []string{"-pid-file", data.PIDFile.ValueString(), "-dhcp6-server", "-d6s.config", data.ConfigFile.ValueString()}
	if res, err := cmd.RunDetached(d, srvCmd, append(srvArgs, moreArgs...)...); err != nil {
		return fmt.Errorf("failed to start DHCPv6 server: %w, diagnostics: %v", err, res.Diagnostics())
	}
	return nil
}

// stopDHCP6Server stops the DHCPv6 server daemon for the given resource
func (r *DHCP6Server) stopDHCP6Server(d string) error {
	running, proc, err := readDHCP6ServerPID(d)
	if err != nil {
		return fmt.Errorf("can't find DHCPv6 server process: %w", err)
	}
	if !running {
		return nil // Already stopped
	}

	var killErr error
	if r.providerConf.UseSudo {
		// Process was started with sudo, so we need sudo to kill it.
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
		return fmt.Errorf("can't kill DHCPv6 server process: %w", killErr)
	}
	return nil
}

func (r *DHCP6Server) readDHCP6Server(resPath string, model *DHCP6ServerModel) (diag.Diagnostics, error) {
	// Check if resource directory exists
	if _, err := os.Stat(resPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("resource directory does not exist")
	}

	// Determine desired state (default to "running" if not set)
	desiredState := "running"
	if !model.State.IsNull() && model.State.ValueString() != "" {
		desiredState = model.State.ValueString()
	}

	// Check if daemon is actually running
	running, _, _ := readDHCP6ServerPID(resPath)
	actualState := "stopped"
	if running {
		actualState = "running"
	}

	// Self-healing: reconcile actual state with desired state
	if desiredState == "running" && actualState == "stopped" {
		// Daemon should be running but isn't - restart it
		tflog.Info(context.Background(), "DHCPv6 server daemon is stopped but should be running, restarting...")
		if err := r.startDHCP6Server(resPath, model); err != nil {
			return nil, fmt.Errorf("failed to restart DHCPv6 server: %w", err)
		}
		actualState = "running"
	} else if desiredState == "stopped" && actualState == "running" {
		// Daemon should be stopped but is running - stop it
		tflog.Info(context.Background(), "DHCPv6 server daemon is running but should be stopped, stopping...")
		if err := r.stopDHCP6Server(resPath); err != nil {
			return nil, fmt.Errorf("failed to stop DHCPv6 server: %w", err)
		}
		actualState = "stopped"
	}

	// Update state to match actual state
	model.State = types.StringValue(actualState)

	return nil, nil
}

func readDHCP6ServerPID(path string) (bool, *process.Process, error) {
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
