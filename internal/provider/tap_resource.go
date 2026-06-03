// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/cmd"
	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/errchecker"
	"github.com/davecgh/go-spew/spew"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

const (
	tapIntfsDir = "tap_intfs"
)

// intfNotFoundStrs is a list of strings that might be present in various
// Linux "show interface" commands and tell us that the specific interface
// is not present.
var intfNotFoundStrs = []string{
	"does not exist",
	"Cannot find device",
	"cannot find device",
	"No such device",
	"no such device",
}

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                = &TAP{}
	_ resource.ResourceWithImportState = &TAP{}
)

func NewTAP() resource.Resource {
	return &TAP{}
}

// TAP defines the resource implementation.
type TAP struct {
	providerConf *ZedAmigoProviderConfig
}

// TAPModel describes the resource data model.
type TAPModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	MTU         types.Int64  `tfsdk:"mtu"`
	State       types.String `tfsdk:"state"`
	Owner       types.String `tfsdk:"owner"`
	Group       types.String `tfsdk:"group"`
	Master      types.String `tfsdk:"master"` // Bridge name to attach to.
	IPv4Address types.String `tfsdk:"ipv4_address"`
	NetNS       types.String `tfsdk:"netns"`
	MoverStatus types.String `tfsdk:"mover_status"` // computed: "pending", "moved", "error", or ""
}

func (r *TAP) getResourceDir(id string) string {
	return filepath.Join(r.providerConf.LibPath, tapIntfsDir, id)
}

func (r *TAP) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tap"
}

func (r *TAP) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "TAP interface",
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Create and manage a Linux network TAP interface using iproute2 commands.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				Description:         "TAP identifier",
				MarkdownDescription: "TAP identifier",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Name of the TAP interface",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"mtu": schema.Int64Attribute{
				Description: "MTU size for the TAP interface",
				Optional:    true,
				Computed:    true,
			},
			"state": schema.StringAttribute{
				Description: "State of the TAP interface (up/down)",
				Optional:    true,
				Computed:    true,
			},
			"owner": schema.StringAttribute{
				Description: "Owner of the TAP interface",
				Optional:    true,
			},
			"group": schema.StringAttribute{
				Description: "Group of the TAP interface",
				Optional:    true,
			},
			"master": schema.StringAttribute{
				Description: "Bridge interface to attach this TAP interface to",
				Optional:    true,
			},
			"ipv4_address": schema.StringAttribute{
				Description: "IPv4 address to configure on the TAP interface",
				Optional:    true,
			},
			"netns": schema.StringAttribute{
				Description: "Network namespace to move this TAP interface into after QEMU opens it",
				Optional:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"mover_status": schema.StringAttribute{
				Computed:    true,
				Description: "Status of the TAP mover daemon: pending, moved, error, or empty",
			},
		},
	}
}

func (r *TAP) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
	tflog.Trace(ctx, "TAP resource configure debugging", traceData)
}

func (r *TAP) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data TAPModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, err := newResourceID()
	if err != nil {
		resp.Diagnostics.AddError("TAP Resource Error",
			fmt.Sprintf("Unable to generate a new resource ID: %s", err))
		return
	}
	data.ID = types.StringValue(id)
	if resp.Diagnostics.HasError() {
		return
	}

	d := r.getResourceDir(data.ID.ValueString())
	if err := os.MkdirAll(d, 0o700); err != nil {
		resp.Diagnostics.AddError("TAP Resource Error",
			fmt.Sprintf("Unable to create resource specific directory: %s", err))
		return
	}
	if err := createTFBackPointer(d); err != nil {
		resp.Diagnostics.AddError("TAP Resource Error",
			fmt.Sprintf("Unable to create resource specific file: %s", err))
		return
	}

	tapIf := data.Name.ValueString()

	// TAP is always created in the default namespace so QEMU can open it.
	ipCmd := r.providerConf.IP
	ipArgs := []string{}
	if r.providerConf.UseSudo {
		ipCmd = r.providerConf.Sudo
		ipArgs = []string{"-n", r.providerConf.IP}
	}

	// Create the TAP.
	moreArgs := []string{"tuntap", "add", "dev", tapIf, "mode", "tap"}
	// moreArgs := []string{"tuntap", "add", "dev", tapIf, "mode", "tap", "multi_queue"}
	if !data.Owner.IsNull() && !data.Owner.IsUnknown() {
		moreArgs = append(moreArgs, "user", data.Owner.ValueString())
	}

	if !data.Group.IsNull() && !data.Group.IsUnknown() {
		moreArgs = append(moreArgs, "group", data.Group.ValueString())
	}
	res, err := cmd.Run(d, ipCmd, append(ipArgs, moreArgs...)...)
	if err != nil {
		resp.Diagnostics.AddError("TAP Resource Error",
			"Unable to create a new TAP.")
		resp.Diagnostics.Append(res.Diagnostics()...)
		return
	}

	// Set the MTU if specified.
	if !data.MTU.IsNull() && !data.MTU.IsUnknown() {
		mtu := fmt.Sprintf("%d", data.MTU.ValueInt64())
		moreArgs := []string{"link", "set", "dev", tapIf, "mtu", mtu}
		res, err := cmd.Run(d, ipCmd, append(ipArgs, moreArgs...)...)
		if err != nil {
			resp.Diagnostics.AddError("TAP Resource Error",
				"Unable to create a new TAP.")
			resp.Diagnostics.Append(res.Diagnostics()...)
			return
		}
	}

	hasNetNS := !data.NetNS.IsNull() && !data.NetNS.IsUnknown()

	if hasNetNS {
		// When netns is set, skip master/state/ipv4 — the mover daemon
		// handles these after moving the TAP into the namespace.
		netns := data.NetNS.ValueString()
		if err := r.startTAPMover(d, &data, netns); err != nil {
			resp.Diagnostics.AddError("TAP Resource Error",
				fmt.Sprintf("Failed to start TAP mover daemon: %v", err))
			return
		}
		data.MoverStatus = types.StringValue("pending")
	} else {
		// No netns — configure master, state, and ipv4 in the default namespace.
		data.MoverStatus = types.StringValue("")

		// Set the master (bridge) if specified.
		if !data.Master.IsNull() && !data.Master.IsUnknown() {
			master := data.Master.ValueString()
			moreArgs := []string{"link", "set", "dev", tapIf, "master", master}
			res, err := cmd.Run(d, ipCmd, append(ipArgs, moreArgs...)...)
			if err != nil {
				resp.Diagnostics.AddError("TAP Resource Error",
					"Unable to create a new TAP.")
				resp.Diagnostics.Append(res.Diagnostics()...)
				return
			}
		}

		// Set the state if specified. NOTE: this MUST be done after setting
		// the master, if master is specified.
		if !data.State.IsNull() && !data.State.IsUnknown() {
			state := data.State.ValueString()
			moreArgs := []string{"link", "set", "dev", tapIf, state}
			res, err := cmd.Run(d, ipCmd, append(ipArgs, moreArgs...)...)
			if err != nil {
				resp.Diagnostics.AddError("TAP Resource Error",
					"Unable to create a new TAP.")
				resp.Diagnostics.Append(res.Diagnostics()...)
				return
			}
		}

		// Configure an IPv4 address if specified.
		if !data.IPv4Address.IsNull() && !data.IPv4Address.IsUnknown() {
			addr := data.IPv4Address.ValueString()

			// Validate the CIDR format
			_, _, err := net.ParseCIDR(addr)
			if err != nil {
				resp.Diagnostics.AddError("Invalid IPv4 address format",
					fmt.Sprintf("IPv4 address must be in CIDR format (e.g., '192.168.1.1/24'): %s", err.Error()))
				return
			}

			moreArgs := []string{"addr", "add", addr, "dev", tapIf}
			res, err := cmd.Run(d, ipCmd, append(ipArgs, moreArgs...)...)
			if err != nil {
				resp.Diagnostics.AddError("TAP Resource Error",
					"Unable to create a new TAP.")
				resp.Diagnostics.Append(res.Diagnostics()...)
				return
			}

		}

		// Read the TAP current state.
		if diags, err := r.readTAP(d, ipCmd, ipArgs, &data); err != nil {
			resp.Diagnostics.AddError("Failed to read TAP state", err.Error())
			resp.Diagnostics.Append(diags...)
			return
		}
	}

	tflog.Trace(ctx, "TAP Resource created succesfully")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *TAP) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data TAPModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d := r.getResourceDir(data.ID.ValueString())

	hasNetNS := !data.NetNS.IsNull() && !data.NetNS.IsUnknown()

	if hasNetNS {
		// Read mover status file to determine where the TAP is.
		moverStatus := r.readMoverStatus(d)
		data.MoverStatus = types.StringValue(moverStatus)

		netns := data.NetNS.ValueString()
		if moverStatus == "moved" {
			// TAP has been moved into the netns — query it there.
			ipCmd, ipArgs := buildIPCommand(r.providerConf, netns)
			if diags, err := r.readTAP(d, ipCmd, ipArgs, &data); err != nil {
				if errchecker.ContainsAny(err, intfNotFoundStrs) || errchecker.DiagsAny(diags, intfNotFoundStrs) {
					resp.State.RemoveResource(ctx)
					return
				}
				resp.Diagnostics.AddError("Failed to read TAP state", err.Error())
				resp.Diagnostics.Append(diags...)
				return
			}
		} else {
			// TAP is still in the default namespace (pending or error).
			ipCmd, ipArgs := buildIPCommand(r.providerConf, "")
			if diags, err := r.readTAP(d, ipCmd, ipArgs, &data); err != nil {
				if errchecker.ContainsAny(err, intfNotFoundStrs) || errchecker.DiagsAny(diags, intfNotFoundStrs) {
					resp.State.RemoveResource(ctx)
					return
				}
				resp.Diagnostics.AddError("Failed to read TAP state", err.Error())
				resp.Diagnostics.Append(diags...)
				return
			}
		}
	} else {
		data.MoverStatus = types.StringValue("")

		ipCmd := r.providerConf.IP
		ipArgs := []string{}
		if r.providerConf.UseSudo {
			ipCmd = r.providerConf.Sudo
			ipArgs = []string{"-n", r.providerConf.IP}
		}

		// Read the TAP current state.
		if diags, err := r.readTAP(d, ipCmd, ipArgs, &data); err != nil {
			// Check for various error messages that indicate the device doesn't exist.
			if errchecker.ContainsAny(err, intfNotFoundStrs) || errchecker.DiagsAny(diags, intfNotFoundStrs) {
				// Resource was deleted outside Terraform: remove from state.
				resp.State.RemoveResource(ctx)
				return
			}

			resp.Diagnostics.AddError("Failed to read TAP state", err.Error())
			resp.Diagnostics.Append(diags...)
			return
		}
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *TAP) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data TAPModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// TODO: What to do here ?

	resp.Diagnostics.AddError("TAP Resource Update Error", "Update is not supported.")
}

func (r *TAP) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data TAPModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tapIf := data.Name.ValueString()
	d := r.getResourceDir(data.ID.ValueString())

	hasNetNS := !data.NetNS.IsNull() && !data.NetNS.IsUnknown()

	// Stop the mover daemon if it's running.
	if hasNetNS {
		r.stopTAPMover(d)
	}

	// Default namespace ip command.
	ipCmd := r.providerConf.IP
	ipArgs := []string{}
	if r.providerConf.UseSudo {
		ipCmd = r.providerConf.Sudo
		ipArgs = []string{"-n", r.providerConf.IP}
	}

	if hasNetNS {
		netns := data.NetNS.ValueString()

		// Try to move the TAP back to the default namespace.
		nsIpCmd, nsIpArgs := buildIPCommand(r.providerConf, netns)
		moveBackArgs := []string{"link", "set", tapIf, "netns", "1"}
		cmd.Run(d, nsIpCmd, append(nsIpArgs, moveBackArgs...)...)
		// Ignore errors — TAP or netns may already be gone.
	}

	// Remove from bridge first if attached (in default namespace).
	if !data.Master.IsNull() {
		moreArgs := []string{"link", "set", "dev", tapIf, "nomaster"}
		res, err := cmd.Run(d, ipCmd, append(ipArgs, moreArgs...)...)
		if err != nil {
			if errchecker.ContainsNone(err, intfNotFoundStrs) &&
				errchecker.DiagsNone(res.Diagnostics(), intfNotFoundStrs) {
				resp.Diagnostics.AddError("Failed to delete TAP", err.Error())
				resp.Diagnostics.Append(res.Diagnostics()...)
				return
			}
		}
	}

	// Delete an existing TAP.
	moreArgs := []string{"tuntap", "delete", "dev", tapIf, "mode", "tap"}
	// moreArgs := []string{"tuntap", "delete", "dev", tapIf, "mode", "tap", "multi_queue"}
	res, err := cmd.Run(d, ipCmd, append(ipArgs, moreArgs...)...)
	if err != nil {
		if errchecker.ContainsNone(err, intfNotFoundStrs) &&
			errchecker.DiagsNone(res.Diagnostics(), intfNotFoundStrs) {
			resp.Diagnostics.AddError("Failed to delete TAP", err.Error())
			resp.Diagnostics.Append(res.Diagnostics()...)
			return
		}
	}

	if err := os.RemoveAll(d); err != nil {
		resp.Diagnostics.AddError("TAP Resource Delete Error",
			fmt.Sprintf("Can't delete TAP resource directory: %v", err))
		return
	}
}

func (r *TAP) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *TAP) readTAP(resPath string, ipCmd string, ipArgs []string, model *TAPModel) (diag.Diagnostics, error) {
	tapIf := model.Name.ValueString()

	// Check if TAP exists and get info.
	moreArgs := []string{"link", "show", tapIf}
	res, err := cmd.Run(resPath, ipCmd, append(ipArgs, moreArgs...)...)
	if err != nil {
		return res.Diagnostics(), fmt.Errorf("can't retrieve TAP '%s' details: %w", tapIf, err)
	}

	// Parse output for MTU and state.
	lines := strings.Split(res.Stdout, "\n")
	// A real `ip link show` line always contains the flags section "<...>".
	// If it doesn't, the output is empty or unparseable: fail loudly with the
	// raw output instead of fabricating null/down values (which would surface
	// as a spurious "inconsistent result after apply" error).
	if len(lines) == 0 || !strings.Contains(lines[0], "<") {
		return res.Diagnostics(),
			fmt.Errorf("can't parse TAP '%s' link details, unexpected output: %q", tapIf, res.Stdout)
	}
	// Parse first line: "2: br0: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 ..."
	mtuRegex := regexp.MustCompile(`mtu (\d+)`)
	if matches := mtuRegex.FindStringSubmatch(lines[0]); len(matches) > 1 {
		x, err := strconv.ParseInt(matches[1], 10, 64)
		if err != nil {
			e := fmt.Errorf("invalid TAP '%s' MTU value '%s': %w", tapIf, matches[1], err)
			d := diag.Diagnostics{}
			d.AddError("Can't find TAP interface MTU value", e.Error())
			return d, e
		}
		model.MTU = types.Int64Value(x)
	}

	// Check for master (bridge).
	masterRegex := regexp.MustCompile(`master (\S+)`)
	if matches := masterRegex.FindStringSubmatch(lines[0]); len(matches) > 1 {
		model.Master = types.StringValue(matches[1])
	} else {
		model.Master = types.StringNull()
	}

	// Determine state from the administrative UP flag inside "<...>" rather
	// than a substring match on the whole line. This reflects the state we
	// actually set, independent of the operational "state DOWN"/NO-CARRIER
	// condition that persists until QEMU opens the TAP.
	if linkFlagUp(lines[0]) {
		model.State = types.StringValue("up")
	} else {
		model.State = types.StringValue("down")
	}

	// Get IP address(es) of TAP.
	moreArgs = []string{"addr", "show", tapIf}
	res, err = cmd.Run(resPath, ipCmd, append(ipArgs, moreArgs...)...)
	if err != nil {
		return res.Diagnostics(), fmt.Errorf("can't retrieve TAP '%s' addreses: %w", tapIf, err)
	}
	// Look for IPv4 address in CIDR format: inet 192.168.1.1/24 brd ...
	addrRegex := regexp.MustCompile(`inet (\d+\.\d+\.\d+\.\d+/\d+)`)
	if matches := addrRegex.FindStringSubmatch(res.Stdout); len(matches) > 1 {
		// Validate using net.ParseCIDR to ensure it's properly formatted
		if _, _, err := net.ParseCIDR(matches[1]); err == nil {
			model.IPv4Address = types.StringValue(matches[1])
		}
	}

	return nil, nil
}

// startTAPMover writes the mover config YAML and starts the mover daemon.
func (r *TAP) startTAPMover(d string, data *TAPModel, netns string) error {
	tapIf := data.Name.ValueString()
	statusFile := filepath.Join(d, "mover_status")
	configPath := filepath.Join(d, "mover_config.yaml")
	pidFile := filepath.Join(d, "mover_pid")

	// Build YAML config.
	master := ""
	if !data.Master.IsNull() && !data.Master.IsUnknown() {
		master = data.Master.ValueString()
	}
	state := ""
	if !data.State.IsNull() && !data.State.IsUnknown() {
		state = data.State.ValueString()
	}

	config := fmt.Sprintf(
		"tap_name: %q\nnetns: %q\nmaster: %q\nstate: %q\nstatus_file: %q\npoll_interval_ms: 500\ntimeout_s: 300\nuse_sudo: %t\nsudo_path: %q\nip_path: %q\n",
		tapIf, netns, master, state, statusFile,
		r.providerConf.UseSudo, r.providerConf.Sudo, r.providerConf.IP,
	)

	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		return fmt.Errorf("can't write mover config: %w", err)
	}

	srvCmd := os.Args[0]
	srvArgs := []string{}
	if r.providerConf.UseSudo {
		srvCmd = r.providerConf.Sudo
		srvArgs = []string{"-n", os.Args[0]}
	}
	moreArgs := []string{"-pid-file", pidFile, "-tap-mover", "-tm.config", configPath}
	if res, err := cmd.RunDetached(d, srvCmd, append(srvArgs, moreArgs...)...); err != nil {
		return fmt.Errorf("failed to start TAP mover: %w, diagnostics: %v", err, res.Diagnostics())
	}
	return nil
}

// stopTAPMover attempts to stop the mover daemon by reading its PID file.
func (r *TAP) stopTAPMover(d string) {
	pidFile := filepath.Join(d, "mover_pid")
	pidBytes, err := os.ReadFile(pidFile)
	if err != nil {
		return
	}
	pid, err := strconv.ParseInt(strings.TrimSpace(string(pidBytes)), 10, 32)
	if err != nil {
		return
	}
	if r.providerConf.UseSudo {
		cmd.Run(d, r.providerConf.Sudo, "-n", "kill", fmt.Sprintf("%d", pid))
	} else {
		p, err := os.FindProcess(int(pid))
		if err == nil {
			p.Kill()
		}
	}
}

// readMoverStatus reads the mover status file and returns the status string.
func (r *TAP) readMoverStatus(d string) string {
	statusFile := filepath.Join(d, "mover_status")
	data, err := os.ReadFile(statusFile)
	if err != nil {
		// Check if mover PID file exists — if so, mover was started but
		// hasn't finished yet.
		pidFile := filepath.Join(d, "mover_pid")
		if _, err := os.Stat(pidFile); err == nil {
			return "pending"
		}
		return ""
	}
	content := strings.TrimSpace(string(data))
	// The status file is JSON: {"status": "moved", ...} or {"status": "error", ...}
	if strings.Contains(content, `"moved"`) {
		return "moved"
	}
	if strings.Contains(content, `"error"`) {
		return "error"
	}
	return "pending"
}
