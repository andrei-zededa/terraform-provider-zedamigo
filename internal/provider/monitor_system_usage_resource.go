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
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/shirou/gopsutil/v4/process"
)

const (
	msuResDir              = "monitor_system_usage"
	msuConfigTemplateText  = `# Monitor system usage configuration (auto-generated)
output_file: {{ .OutputFile }}
interval: "{{ .Interval }}"
flush_every_n: {{ .FlushEveryN }}
include_env: "{{ .IncludeEnv }}"
namespaces:
{{- range .Namespaces }}
  - "{{ . }}"
{{- end }}
`
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                = &MonitorSystemUsage{}
	_ resource.ResourceWithImportState = &MonitorSystemUsage{}
)

func NewMonitorSystemUsage() resource.Resource {
	return &MonitorSystemUsage{}
}

// MonitorSystemUsage defines the resource implementation.
type MonitorSystemUsage struct {
	providerConf *ZedAmigoProviderConfig
}

// MonitorSystemUsageModel describes the resource data model.
type MonitorSystemUsageModel struct {
	ID          types.String `tfsdk:"id"`
	Interval    types.String `tfsdk:"interval"`
	FlushEveryN types.Int64  `tfsdk:"flush_every_n"`
	Namespaces  types.List   `tfsdk:"namespaces"`
	IncludeEnv  types.String `tfsdk:"include_env"`
	OutputFile  types.String `tfsdk:"output_file"`
	ConfigFile  types.String `tfsdk:"config_file"`
	PIDFile     types.String `tfsdk:"pid_file"`
	State       types.String `tfsdk:"state"`
	NetNS       types.String `tfsdk:"netns"`
}

func (r *MonitorSystemUsage) getResourceDir(id string) string {
	return filepath.Join(r.providerConf.LibPath, msuResDir, id)
}

func (r *MonitorSystemUsage) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_monitor_system_usage"
}

func (r *MonitorSystemUsage) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Periodic msu-collect-style system monitor writing MSU CBOR v2 samples",
		MarkdownDescription: undent.Md(`Create and manage an embedded |msu-collect| daemon that periodically samples Linux system and per-interface
		state (/proc, /sys, ethtool, iptables, etc.) and writes the data as an MSU CBOR v2 file — the same format the upstream
		|msu-collect| binary produces. The output is readable by the |msu-analyst| tool and any other MSU v2 consumer.

		The daemon runs an A-section (heavyweight) every 3 intervals and a B-section (lightweight) every interval, plus a one-time
		init-section at startup. Each (section, command, namespace) tuple becomes a Sample stream in the CBOR file.

		NOTE: many of the A-section commands (iptables, dmidecode, …) need root to produce useful output. The collector degrades
		gracefully — failed commands are recorded in the sample's |err| field. Set the provider |use_sudo = true| if you want
		the daemon wrapped in |sudo|.`),

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				Description:         "Monitor system usage resource identifier",
				MarkdownDescription: "Monitor system usage resource identifier.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"interval": schema.StringAttribute{
				Description:         "Collection interval (Go duration string, e.g. \"10s\", \"30s\")",
				MarkdownDescription: "Collection interval as a Go duration string (e.g. |10s|, |30s|). Default: |10s|.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("10s"),
			},
			"flush_every_n": schema.Int64Attribute{
				Description: "Flush the MSU CBOR output file every N collection intervals",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(6),
			},
			"namespaces": schema.ListAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "Network namespaces to monitor (msu-collect -n equivalent)",
				MarkdownDescription: undent.Md(`List of network namespace names to ALSO monitor (equivalent to |msu-collect -n ns1,ns2|).
				For each namespace the collector additionally captures |/proc/net/*|, iptables, and per-interface ethtool/tc/ip output
				via |ip netns exec <ns>|. Leave empty (default) to monitor only the root namespace.

				This is independent of the |netns| attribute below, which controls where the daemon itself RUNS.`),
			},
			"include_env": schema.StringAttribute{
				Description:         "Environment variables to record in the header: filtered | all | none",
				MarkdownDescription: "Environment variables to record in the header. Allowed values: |filtered| (default, drops tokens/keys/secrets/passwords/auth/credentials/cookies by name), |all| (no filtering), |none| (omit environment).",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("filtered"),
			},
			"output_file": schema.StringAttribute{
				Description:         "Path to the MSU CBOR output file",
				MarkdownDescription: "Path to the MSU CBOR output file. If omitted, defaults to |<resource_dir>/data.msu.cbor|.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"config_file": schema.StringAttribute{
				Computed:    true,
				Description: "The auto-generated monitor-system-usage configuration file",
			},
			"pid_file": schema.StringAttribute{
				Computed:    true,
				Description: "Process ID file",
			},
			"state": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Desired state of the monitor-system-usage daemon",
				MarkdownDescription: undent.Md(`Desired state of the monitor-system-usage daemon. Can be |running| or |stopped|.
				Defaults to |running|. The provider will automatically start or stop the daemon to match this state.`),
			},
			"netns": schema.StringAttribute{
				Description: "Network namespace in which to run the monitor-system-usage daemon",
				Optional:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *MonitorSystemUsage) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
	tflog.Trace(ctx, "Monitor system usage resource configure debugging", traceData)
}

func (r *MonitorSystemUsage) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data MonitorSystemUsageModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, err := newResourceID()
	if err != nil {
		resp.Diagnostics.AddError("MonitorSystemUsage Resource Error",
			fmt.Sprintf("Unable to generate a new resource ID: %s", err))
		return
	}
	data.ID = types.StringValue(id)

	d := r.getResourceDir(data.ID.ValueString())
	if err := os.MkdirAll(d, 0o700); err != nil {
		resp.Diagnostics.AddError("MonitorSystemUsage Resource Error",
			fmt.Sprintf("Unable to create resource specific directory: %s", err))
		return
	}
	if err := createTFBackPointer(d); err != nil {
		resp.Diagnostics.AddError("MonitorSystemUsage Resource Error",
			fmt.Sprintf("Unable to create resource specific file: %s", err))
		return
	}

	if data.OutputFile.IsNull() || data.OutputFile.IsUnknown() || data.OutputFile.ValueString() == "" {
		data.OutputFile = types.StringValue(filepath.Join(d, "data.msu.cbor"))
	}

	var namespaces []string
	if !data.Namespaces.IsNull() && !data.Namespaces.IsUnknown() {
		diags := data.Namespaces.ElementsAs(ctx, &namespaces, false)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	confPath := filepath.Join(d, "config.yaml")
	if err := writeMSUConfig(confPath, &data, namespaces); err != nil {
		resp.Diagnostics.AddError("MonitorSystemUsage Resource Error",
			fmt.Sprintf("Unable to write config file: %s", err))
		return
	}
	data.ConfigFile = types.StringValue(confPath)

	pidFile := filepath.Join(d, "pid")
	data.PIDFile = types.StringValue(pidFile)

	if data.State.IsNull() || data.State.ValueString() == "" {
		data.State = types.StringValue("running")
	}

	if data.State.ValueString() == "running" {
		if err := r.startMonitorSystemUsage(d, &data); err != nil {
			resp.Diagnostics.AddError("MonitorSystemUsage Resource Error",
				fmt.Sprintf("Failed to start monitor-system-usage daemon: %v", err))
			return
		}
	}

	if diags, err := r.readMonitorSystemUsage(d, &data); err != nil {
		resp.Diagnostics.AddError("Failed to read MonitorSystemUsage state", err.Error())
		resp.Diagnostics.Append(diags...)
		return
	}

	tflog.Trace(ctx, "MonitorSystemUsage Resource created successfully")

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *MonitorSystemUsage) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data MonitorSystemUsageModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d := r.getResourceDir(data.ID.ValueString())

	if diags, err := r.readMonitorSystemUsage(d, &data); err != nil {
		if strings.Contains(err.Error(), "does not exist") {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read MonitorSystemUsage state", err.Error())
		resp.Diagnostics.Append(diags...)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *MonitorSystemUsage) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan MonitorSystemUsageModel
	var state MonitorSystemUsageModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d := r.getResourceDir(state.ID.ValueString())

	stateChanged := !plan.State.Equal(state.State)
	configChanged := !plan.Interval.Equal(state.Interval) ||
		!plan.FlushEveryN.Equal(state.FlushEveryN) ||
		!plan.Namespaces.Equal(state.Namespaces) ||
		!plan.IncludeEnv.Equal(state.IncludeEnv) ||
		!plan.OutputFile.Equal(state.OutputFile)

	if configChanged {
		resp.Diagnostics.AddError("MonitorSystemUsage Resource Update Error",
			"Configuration changes require resource recreation. Only the 'state' field can be updated in-place.")
		return
	}

	plan.ConfigFile = state.ConfigFile
	plan.PIDFile = state.PIDFile
	plan.OutputFile = state.OutputFile

	if stateChanged {
		desiredState := plan.State.ValueString()
		if desiredState == "" {
			desiredState = "running"
		}

		tflog.Info(ctx, "Monitor system usage state change requested", map[string]any{
			"from": state.State.ValueString(),
			"to":   desiredState,
		})

		if desiredState == "running" {
			if err := r.startMonitorSystemUsage(d, &plan); err != nil {
				resp.Diagnostics.AddError("MonitorSystemUsage Resource Update Error",
					fmt.Sprintf("Failed to start monitor-system-usage daemon: %v", err))
				return
			}
		} else if desiredState == "stopped" {
			if err := r.stopMonitorSystemUsage(d); err != nil {
				resp.Diagnostics.AddError("MonitorSystemUsage Resource Update Error",
					fmt.Sprintf("Failed to stop monitor-system-usage daemon: %v", err))
				return
			}
		}

		plan.State = types.StringValue(desiredState)
	}

	if diags, err := r.readMonitorSystemUsage(d, &plan); err != nil {
		resp.Diagnostics.AddError("Failed to read MonitorSystemUsage state after update", err.Error())
		resp.Diagnostics.Append(diags...)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *MonitorSystemUsage) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data MonitorSystemUsageModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d := r.getResourceDir(data.ID.ValueString())

	if err := r.stopMonitorSystemUsage(d); err != nil {
		tflog.Warn(ctx, "Failed to stop monitor-system-usage daemon during delete", map[string]any{"error": err.Error()})
	}

	if err := os.RemoveAll(d); err != nil {
		resp.Diagnostics.AddError("MonitorSystemUsage Resource Delete Error",
			fmt.Sprintf("Can't delete MonitorSystemUsage resource directory: %v", err))
		return
	}
}

func (r *MonitorSystemUsage) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *MonitorSystemUsage) startMonitorSystemUsage(d string, data *MonitorSystemUsageModel) error {
	netns := ""
	if !data.NetNS.IsNull() && !data.NetNS.IsUnknown() {
		netns = data.NetNS.ValueString()
	}

	srvCmd := os.Args[0]
	srvArgs := []string{}
	if netns != "" {
		if r.providerConf.UseSudo {
			srvCmd = r.providerConf.Sudo
			srvArgs = []string{"-n", r.providerConf.IP, "netns", "exec", netns, os.Args[0]}
		} else {
			srvCmd = r.providerConf.IP
			srvArgs = []string{"netns", "exec", netns, os.Args[0]}
		}
	} else if r.providerConf.UseSudo {
		srvCmd = r.providerConf.Sudo
		srvArgs = []string{"-n", os.Args[0]}
	}
	moreArgs := []string{"-pid-file", data.PIDFile.ValueString(), "-monitor-system-usage", "-msu.config", data.ConfigFile.ValueString()}
	if res, err := cmd.RunDetached(d, srvCmd, append(srvArgs, moreArgs...)...); err != nil {
		return fmt.Errorf("failed to start monitor-system-usage daemon: %w, diagnostics: %v", err, res.Diagnostics())
	}
	return nil
}

func (r *MonitorSystemUsage) stopMonitorSystemUsage(d string) error {
	running, proc, err := readMonitorSystemUsagePID(d)
	if err != nil {
		return fmt.Errorf("can't find monitor-system-usage daemon process: %w", err)
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
		return fmt.Errorf("can't kill monitor-system-usage daemon process: %w", killErr)
	}
	return nil
}

func (r *MonitorSystemUsage) readMonitorSystemUsage(resPath string, model *MonitorSystemUsageModel) (diag.Diagnostics, error) {
	if _, err := os.Stat(resPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("resource directory does not exist")
	}

	desiredState := "running"
	if !model.State.IsNull() && model.State.ValueString() != "" {
		desiredState = model.State.ValueString()
	}

	running, _, _ := readMonitorSystemUsagePID(resPath)
	actualState := "stopped"
	if running {
		actualState = "running"
	}

	if desiredState == "running" && actualState == "stopped" {
		tflog.Info(context.Background(), "Monitor system usage daemon is stopped but should be running, restarting...")
		if err := r.startMonitorSystemUsage(resPath, model); err != nil {
			return nil, fmt.Errorf("failed to restart monitor-system-usage daemon: %w", err)
		}
		actualState = "running"
	} else if desiredState == "stopped" && actualState == "running" {
		tflog.Info(context.Background(), "Monitor system usage daemon is running but should be stopped, stopping...")
		if err := r.stopMonitorSystemUsage(resPath); err != nil {
			return nil, fmt.Errorf("failed to stop monitor-system-usage daemon: %w", err)
		}
		actualState = "stopped"
	}

	model.State = types.StringValue(actualState)

	return nil, nil
}

func readMonitorSystemUsagePID(path string) (bool, *process.Process, error) {
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

func writeMSUConfig(confPath string, data *MonitorSystemUsageModel, namespaces []string) error {
	tmpl, err := template.New("msu-config").Parse(msuConfigTemplateText)
	if err != nil {
		return fmt.Errorf("parse template: %w", err)
	}

	confFile, err := os.Create(confPath)
	if err != nil {
		return fmt.Errorf("create %s: %w", confPath, err)
	}
	defer confFile.Close()

	td := struct {
		OutputFile  string
		Interval    string
		FlushEveryN int64
		IncludeEnv  string
		Namespaces  []string
	}{
		OutputFile:  data.OutputFile.ValueString(),
		Interval:    data.Interval.ValueString(),
		FlushEveryN: data.FlushEveryN.ValueInt64(),
		IncludeEnv:  data.IncludeEnv.ValueString(),
		Namespaces:  namespaces,
	}
	if err := tmpl.Execute(confFile, td); err != nil {
		return fmt.Errorf("write %s: %w", confPath, err)
	}
	return nil
}
