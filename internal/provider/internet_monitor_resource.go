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
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

const (
	imDir                = "internet_monitors"
	imConfigTemplateText = `# Internet monitor configuration (auto-generated)
output_file: {{ .OutputFile }}
interval: "{{ .Interval }}"
ping_count: {{ .PingCount }}
ping_timeout: "{{ .PingTimeout }}"
dns_timeout: "{{ .DNSTimeout }}"
http_timeout: "{{ .HTTPTimeout }}"
doh_endpoint: "{{ .DoHEndpoint }}"
flush_every_n: {{ .FlushEveryN }}
privileged_icmp: {{ .PrivilegedICMP }}
destinations:
{{- range .Destinations }}
  - "{{ . }}"
{{- end }}
`
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                = &InternetMonitor{}
	_ resource.ResourceWithImportState = &InternetMonitor{}
)

func NewInternetMonitor() resource.Resource {
	return &InternetMonitor{}
}

// InternetMonitor defines the resource implementation.
type InternetMonitor struct {
	providerConf *ZedAmigoProviderConfig
}

// InternetMonitorModel describes the resource data model.
type InternetMonitorModel struct {
	ID             types.String `tfsdk:"id"`
	Destinations   types.List   `tfsdk:"destinations"`
	Interval       types.String `tfsdk:"interval"`
	PingCount      types.Int64  `tfsdk:"ping_count"`
	PingTimeout    types.String `tfsdk:"ping_timeout"`
	DNSTimeout     types.String `tfsdk:"dns_timeout"`
	HTTPTimeout    types.String `tfsdk:"http_timeout"`
	DoHEndpoint    types.String `tfsdk:"doh_endpoint"`
	FlushEveryN    types.Int64  `tfsdk:"flush_every_n"`
	PrivilegedICMP types.Bool   `tfsdk:"privileged_icmp"`
	OutputFile     types.String `tfsdk:"output_file"`
	ConfigFile     types.String `tfsdk:"config_file"`
	PIDFile        types.String `tfsdk:"pid_file"`
	State          types.String `tfsdk:"state"`
	NetNS          types.String `tfsdk:"netns"`
}

func (r *InternetMonitor) getResourceDir(id string) string {
	return filepath.Join(r.providerConf.LibPath, imDir, id)
}

func (r *InternetMonitor) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_internet_monitor"
}

func (r *InternetMonitor) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Periodic internet connectivity probe writing MSU CBOR v2 samples",
		MarkdownDescription: undent.Md(`Create and manage an internet-monitor daemon that periodically probes a list of
		destination URLs. For each destination it runs two rounds — first using the system resolver, then using DNS-over-HTTPS
		via Quad9 — and for each round records:
		  1. DNS resolution (all returned IPv4 + IPv6 addresses, RTT),
		  2. ICMP ping (configurable packet count) to each resolved IP,
		  3. HTTPS GET against the original URL and against each resolved IP with the Host header set to the original hostname.

		Each probe step is written as a separate sample in an |.msu.cbor| file (MSU CBOR format version 2 — see the
		|monitor-system-usage| project) so the data is readable by the |msu-analyst| tool.

		NOTE: unprivileged ICMP requires |net.ipv4.ping_group_range| on the host to include the running user's GID, or set
		|privileged_icmp = true| and run with appropriate privileges.`),

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				Description:         "Internet monitor resource identifier",
				MarkdownDescription: "Internet monitor resource identifier.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"destinations": schema.ListAttribute{
				Required:    true,
				ElementType: types.StringType,
				Description: "URLs to probe (must be HTTP or HTTPS URLs)",
				MarkdownDescription: undent.Md(`List of URLs to probe. Each entry must be a valid HTTP/HTTPS URL,
				e.g. |https://www.google.com|. The hostname is extracted for DNS resolution; the full URL is used for
				HTTPS GET requests.`),
				Validators: []validator.List{
					listvalidator.SizeAtLeast(1),
				},
			},
			"interval": schema.StringAttribute{
				Description:         "Probe cycle interval (Go duration string, e.g. \"60s\", \"5m\")",
				MarkdownDescription: "Probe cycle interval as a Go duration string (e.g. |60s|, |5m|). Default: |60s|.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("60s"),
			},
			"ping_count": schema.Int64Attribute{
				Description: "Number of ICMP echo packets to send per target",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(5),
			},
			"ping_timeout": schema.StringAttribute{
				Description: "Overall timeout for one ICMP probe (Go duration string)",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("5s"),
			},
			"dns_timeout": schema.StringAttribute{
				Description: "Per-query DNS timeout (Go duration string)",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("5s"),
			},
			"http_timeout": schema.StringAttribute{
				Description: "Per-request HTTPS timeout (Go duration string)",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("10s"),
			},
			"doh_endpoint": schema.StringAttribute{
				Description: "DNS-over-HTTPS endpoint URL used for the second resolver round",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("https://dns.quad9.net/dns-query"),
			},
			"flush_every_n": schema.Int64Attribute{
				Description: "Flush the MSU CBOR output file every N probe cycles",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(1),
			},
			"privileged_icmp": schema.BoolAttribute{
				Description: "Use raw-socket ICMP (requires root or CAP_NET_RAW). Default false — uses Linux datagram sockets (\"unprivileged\" ping) which requires net.ipv4.ping_group_range to include the running GID.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"output_file": schema.StringAttribute{
				Description:         "Path to the MSU CBOR output file",
				MarkdownDescription: "Path to the MSU CBOR output file. If omitted, defaults to |<resource_dir>/output.msu.cbor|.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"config_file": schema.StringAttribute{
				Computed:    true,
				Description: "The auto-generated internet-monitor configuration file",
			},
			"pid_file": schema.StringAttribute{
				Computed:    true,
				Description: "Process ID file",
			},
			"state": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Desired state of the internet-monitor daemon",
				MarkdownDescription: undent.Md(`Desired state of the internet-monitor daemon. Can be |running| or |stopped|.
				Defaults to |running|. The provider will automatically start or stop the daemon to match this state.`),
			},
			"netns": schema.StringAttribute{
				Description: "Network namespace in which to run the internet-monitor daemon",
				Optional:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *InternetMonitor) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
	tflog.Trace(ctx, "Internet monitor resource configure debugging", traceData)
}

func (r *InternetMonitor) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data InternetMonitorModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, err := newResourceID()
	if err != nil {
		resp.Diagnostics.AddError("InternetMonitor Resource Error",
			fmt.Sprintf("Unable to generate a new resource ID: %s", err))
		return
	}
	data.ID = types.StringValue(id)

	d := r.getResourceDir(data.ID.ValueString())
	if err := r.providerConf.Exec.MkdirAll(ctx, d, 0o700); err != nil {
		resp.Diagnostics.AddError("InternetMonitor Resource Error",
			fmt.Sprintf("Unable to create resource specific directory: %s", err))
		return
	}
	if err := createTFBackPointer(ctx, r.providerConf.Exec, d); err != nil {
		resp.Diagnostics.AddError("InternetMonitor Resource Error",
			fmt.Sprintf("Unable to create resource specific file: %s", err))
		return
	}

	// Default output_file to <resource_dir>/output.msu.cbor.
	if data.OutputFile.IsNull() || data.OutputFile.IsUnknown() || data.OutputFile.ValueString() == "" {
		data.OutputFile = types.StringValue(filepath.Join(d, "output.msu.cbor"))
	}

	destinations, diags := extractStringList(ctx, data.Destinations)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	confPath := filepath.Join(d, "config.yaml")
	if err := writeIMConfig(ctx, r.providerConf.Exec, confPath, &data, destinations); err != nil {
		resp.Diagnostics.AddError("InternetMonitor Resource Error",
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
		if err := r.startInternetMonitor(ctx, d, &data); err != nil {
			resp.Diagnostics.AddError("InternetMonitor Resource Error",
				fmt.Sprintf("Failed to start internet-monitor daemon: %v", err))
			return
		}
	}

	if diags, err := r.readInternetMonitor(ctx, d, &data); err != nil {
		resp.Diagnostics.AddError("Failed to read InternetMonitor state", err.Error())
		resp.Diagnostics.Append(diags...)
		return
	}

	tflog.Trace(ctx, "InternetMonitor Resource created successfully")

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *InternetMonitor) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data InternetMonitorModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d := r.getResourceDir(data.ID.ValueString())

	if diags, err := r.readInternetMonitor(ctx, d, &data); err != nil {
		if strings.Contains(err.Error(), "does not exist") {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read InternetMonitor state", err.Error())
		resp.Diagnostics.Append(diags...)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *InternetMonitor) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan InternetMonitorModel
	var state InternetMonitorModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d := r.getResourceDir(state.ID.ValueString())

	stateChanged := !plan.State.Equal(state.State)
	configChanged := !plan.Destinations.Equal(state.Destinations) ||
		!plan.Interval.Equal(state.Interval) ||
		!plan.PingCount.Equal(state.PingCount) ||
		!plan.PingTimeout.Equal(state.PingTimeout) ||
		!plan.DNSTimeout.Equal(state.DNSTimeout) ||
		!plan.HTTPTimeout.Equal(state.HTTPTimeout) ||
		!plan.DoHEndpoint.Equal(state.DoHEndpoint) ||
		!plan.FlushEveryN.Equal(state.FlushEveryN) ||
		!plan.PrivilegedICMP.Equal(state.PrivilegedICMP) ||
		!plan.OutputFile.Equal(state.OutputFile)

	if configChanged {
		resp.Diagnostics.AddError("InternetMonitor Resource Update Error",
			"Configuration changes require resource recreation. Only the 'state' field can be updated in-place.")
		return
	}

	// Preserve computed fields.
	plan.ConfigFile = state.ConfigFile
	plan.PIDFile = state.PIDFile
	plan.OutputFile = state.OutputFile

	if stateChanged {
		desiredState := plan.State.ValueString()
		if desiredState == "" {
			desiredState = "running"
		}

		tflog.Info(ctx, "Internet monitor state change requested", map[string]any{
			"from": state.State.ValueString(),
			"to":   desiredState,
		})

		if desiredState == "running" {
			if err := r.startInternetMonitor(ctx, d, &plan); err != nil {
				resp.Diagnostics.AddError("InternetMonitor Resource Update Error",
					fmt.Sprintf("Failed to start internet-monitor daemon: %v", err))
				return
			}
		} else if desiredState == "stopped" {
			if err := r.stopInternetMonitor(ctx, d); err != nil {
				resp.Diagnostics.AddError("InternetMonitor Resource Update Error",
					fmt.Sprintf("Failed to stop internet-monitor daemon: %v", err))
				return
			}
		}

		plan.State = types.StringValue(desiredState)
	}

	if diags, err := r.readInternetMonitor(ctx, d, &plan); err != nil {
		resp.Diagnostics.AddError("Failed to read InternetMonitor state after update", err.Error())
		resp.Diagnostics.Append(diags...)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *InternetMonitor) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data InternetMonitorModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d := r.getResourceDir(data.ID.ValueString())

	if err := r.stopInternetMonitor(ctx, d); err != nil {
		tflog.Warn(ctx, "Failed to stop internet-monitor daemon during delete", map[string]any{"error": err.Error()})
	}

	if err := r.providerConf.Exec.Remove(ctx, d); err != nil {
		resp.Diagnostics.AddError("InternetMonitor Resource Delete Error",
			fmt.Sprintf("Can't delete InternetMonitor resource directory: %v", err))
		return
	}
}

func (r *InternetMonitor) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// startInternetMonitor starts the internet-monitor daemon for the given resource.
func (r *InternetMonitor) startInternetMonitor(ctx context.Context, d string, data *InternetMonitorModel) error {
	netns := ""
	if !data.NetNS.IsNull() && !data.NetNS.IsUnknown() {
		netns = data.NetNS.ValueString()
	}

	self := r.providerConf.Exec.SelfPath()
	srvCmd := self
	srvArgs := []string{}
	if netns != "" {
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
	moreArgs := []string{"-pid-file", data.PIDFile.ValueString(), "-internet-monitor", "-im.config", data.ConfigFile.ValueString()}
	if res, err := r.providerConf.Exec.RunDetached(ctx, d, srvCmd, append(srvArgs, moreArgs...)...); err != nil {
		return fmt.Errorf("failed to start internet-monitor daemon: %w, diagnostics: %v", err, res.Diagnostics())
	}
	return nil
}

// stopInternetMonitor stops the internet-monitor daemon for the given resource.
func (r *InternetMonitor) stopInternetMonitor(ctx context.Context, d string) error {
	running, pid, err := readInternetMonitorPID(ctx, r.providerConf.Exec, d)
	if err != nil {
		return fmt.Errorf("can't find internet-monitor daemon process: %w", err)
	}
	if !running {
		return nil
	}

	if err := r.providerConf.Exec.Kill(ctx, pid, syscall.SIGKILL); err != nil {
		return fmt.Errorf("can't kill internet-monitor daemon process: %w", err)
	}
	return nil
}

func (r *InternetMonitor) readInternetMonitor(ctx context.Context, resPath string, model *InternetMonitorModel) (diag.Diagnostics, error) {
	if _, err := r.providerConf.Exec.Stat(ctx, resPath); exec.IsNotExist(err) {
		return nil, fmt.Errorf("resource directory does not exist")
	}

	desiredState := "running"
	if !model.State.IsNull() && model.State.ValueString() != "" {
		desiredState = model.State.ValueString()
	}

	running, _, _ := readInternetMonitorPID(ctx, r.providerConf.Exec, resPath)
	actualState := "stopped"
	if running {
		actualState = "running"
	}

	if desiredState == "running" && actualState == "stopped" {
		tflog.Info(ctx, "Internet monitor daemon is stopped but should be running, restarting...")
		if err := r.startInternetMonitor(ctx, resPath, model); err != nil {
			return nil, fmt.Errorf("failed to restart internet-monitor daemon: %w", err)
		}
		actualState = "running"
	} else if desiredState == "stopped" && actualState == "running" {
		tflog.Info(ctx, "Internet monitor daemon is running but should be stopped, stopping...")
		if err := r.stopInternetMonitor(ctx, resPath); err != nil {
			return nil, fmt.Errorf("failed to stop internet-monitor daemon: %w", err)
		}
		actualState = "stopped"
	}

	model.State = types.StringValue(actualState)

	return nil, nil
}

func readInternetMonitorPID(ctx context.Context, ex exec.Executor, path string) (bool, int, error) {
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

func writeIMConfig(ctx context.Context, ex exec.Executor, confPath string, data *InternetMonitorModel, destinations []string) error {
	tmpl, err := template.New("im-config").Parse(imConfigTemplateText)
	if err != nil {
		return fmt.Errorf("parse template: %w", err)
	}

	confFile, err := ex.OpenWrite(ctx, confPath, 0o644)
	if err != nil {
		return fmt.Errorf("create %s: %w", confPath, err)
	}
	defer confFile.Close()

	td := struct {
		OutputFile     string
		Interval       string
		PingCount      int64
		PingTimeout    string
		DNSTimeout     string
		HTTPTimeout    string
		DoHEndpoint    string
		FlushEveryN    int64
		PrivilegedICMP bool
		Destinations   []string
	}{
		OutputFile:     data.OutputFile.ValueString(),
		Interval:       data.Interval.ValueString(),
		PingCount:      data.PingCount.ValueInt64(),
		PingTimeout:    data.PingTimeout.ValueString(),
		DNSTimeout:     data.DNSTimeout.ValueString(),
		HTTPTimeout:    data.HTTPTimeout.ValueString(),
		DoHEndpoint:    data.DoHEndpoint.ValueString(),
		FlushEveryN:    data.FlushEveryN.ValueInt64(),
		PrivilegedICMP: data.PrivilegedICMP.ValueBool(),
		Destinations:   destinations,
	}
	if err := tmpl.Execute(confFile, td); err != nil {
		return fmt.Errorf("write %s: %w", confPath, err)
	}
	return nil
}

func extractStringList(ctx context.Context, l types.List) ([]string, diag.Diagnostics) {
	var out []string
	diags := l.ElementsAs(ctx, &out, false)
	return out, diags
}
