// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/errchecker"
	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/lladdr"
	"github.com/davecgh/go-spew/spew"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

const (
	bridgesDir = "bridges"
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                = &Bridge{}
	_ resource.ResourceWithImportState = &Bridge{}
)

func NewBridge() resource.Resource {
	return &Bridge{}
}

// Bridge defines the resource implementation.
type Bridge struct {
	providerConf *ZedAmigoProviderConfig
}

// BridgeModel describes the resource data model.
type BridgeModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	MTU         types.Int64  `tfsdk:"mtu"`
	State       types.String `tfsdk:"state"`
	MACAddress  types.String `tfsdk:"mac_address"`
	IPv4Address types.String `tfsdk:"ipv4_address"`
	IPv6Address types.String `tfsdk:"ipv6_address"`
	NetNS       types.String `tfsdk:"netns"`
	// EnslavedInterfaces is the set of existing interfaces to attach as
	// members (slaves) of this bridge.
	EnslavedInterfaces types.Set `tfsdk:"enslaved_interfaces"`
}

func (r *Bridge) getResourceDir(id string) string {
	return filepath.Join(r.providerConf.LibPath, bridgesDir, id)
}

func (r *Bridge) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_bridge"
}

func (r *Bridge) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Bridge",
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Create and manage a Linux network bridge interface using iproute2 commands.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				Description:         "Bridge identifier",
				MarkdownDescription: "Bridge identifier",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Name of the bridge interface",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"mtu": schema.Int64Attribute{
				Description: "MTU size for the bridge",
				Optional:    true,
				Computed:    true,
			},
			"state": schema.StringAttribute{
				Description: "State of the bridge (up/down)",
				Optional:    true,
				Computed:    true,
			},
			"mac_address": schema.StringAttribute{
				Description: "MAC address for the bridge",
				Optional:    true,
				Computed:    true,
			},
			"ipv4_address": schema.StringAttribute{
				Description: "IPv4 address for the bridge",
				Optional:    true,
			},
			"ipv6_address": schema.StringAttribute{
				Description: "IPv6 address for the bridge",
				Optional:    true,
			},
			"netns": schema.StringAttribute{
				Description: "Network namespace in which to create the bridge",
				Optional:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"enslaved_interfaces": schema.SetAttribute{
				Description: "Names of existing interfaces to enslave (attach as members) to this bridge. " +
					"Each interface is attached with `ip link set dev <interface> master <bridge>` and brought up. " +
					"The interfaces must already exist in the same network namespace as the bridge. " +
					"Changing this set requires the bridge to be re-created. " +
					"Only list interfaces that are not otherwise managed as bridge members: do not include " +
					"interfaces (such as `zedamigo_tap` resources) that attach themselves via their own `master` " +
					"attribute, as those are owned by the other resource and are deliberately ignored here.",
				Optional:    true,
				ElementType: types.StringType,
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *Bridge) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
	tflog.Trace(ctx, "Bridge resource configure debugging", traceData)
}

func (r *Bridge) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data BridgeModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, err := newResourceID()
	if err != nil {
		resp.Diagnostics.AddError("Bridge Resource Error",
			fmt.Sprintf("Unable to generate a new resource ID: %s", err))
		return
	}
	data.ID = types.StringValue(id)
	if resp.Diagnostics.HasError() {
		return
	}

	d := r.getResourceDir(data.ID.ValueString())
	if err := r.providerConf.Exec.MkdirAll(ctx, d, 0o700); err != nil {
		resp.Diagnostics.AddError("Bridge Resource Error",
			fmt.Sprintf("Unable to create resource specific directory: %s", err))
		return
	}
	if err := createTFBackPointer(ctx, r.providerConf.Exec, d); err != nil {
		resp.Diagnostics.AddError("Bridge Resource Error",
			fmt.Sprintf("Unable to create resource specific file: %s", err))
		return
	}

	br := data.Name.ValueString()

	netns := ""
	if !data.NetNS.IsNull() && !data.NetNS.IsUnknown() {
		netns = data.NetNS.ValueString()
	}
	ipCmd, ipArgs := buildIPCommand(r.providerConf, netns)

	// Create the bridge.
	moreArgs := []string{"link", "add", br, "type", "bridge"}
	res, err := r.providerConf.Exec.Run(ctx, d, ipCmd, append(ipArgs, moreArgs...)...)
	if err != nil {
		resp.Diagnostics.AddError("Bridge Resource Error",
			"Unable to create a new bridge.")
		resp.Diagnostics.Append(res.Diagnostics()...)
		return
	}

	// Set the MTU if specified.
	if !data.MTU.IsNull() && !data.MTU.IsUnknown() {
		mtu := fmt.Sprintf("%d", data.MTU.ValueInt64())
		moreArgs := []string{"link", "set", "dev", br, "mtu", mtu}
		res, err := r.providerConf.Exec.Run(ctx, d, ipCmd, append(ipArgs, moreArgs...)...)
		if err != nil {
			resp.Diagnostics.AddError("Bridge Resource Error",
				"Unable to create a new bridge.")
			resp.Diagnostics.Append(res.Diagnostics()...)
			return
		}
	}

	// Set the state if specified.
	if !data.State.IsNull() && !data.State.IsUnknown() {
		state := data.State.ValueString()
		moreArgs := []string{"link", "set", "dev", br, state}
		res, err := r.providerConf.Exec.Run(ctx, d, ipCmd, append(ipArgs, moreArgs...)...)
		if err != nil {
			resp.Diagnostics.AddError("Bridge Resource Error",
				"Unable to create a new bridge.")
			resp.Diagnostics.Append(res.Diagnostics()...)
			return
		}
	}

	// Set the MAC address if specified.
	if !data.MACAddress.IsNull() && !data.MACAddress.IsUnknown() {
		macAddr := data.MACAddress.ValueString()
		moreArgs := []string{"link", "set", "dev", br, "address", macAddr}
		res, err := r.providerConf.Exec.Run(ctx, d, ipCmd, append(ipArgs, moreArgs...)...)
		if err != nil {
			resp.Diagnostics.AddError("Bridge Resource Error",
				"Unable to set MAC address for bridge.")
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

		moreArgs := []string{"addr", "add", addr, "dev", br}
		res, err := r.providerConf.Exec.Run(ctx, d, ipCmd, append(ipArgs, moreArgs...)...)
		if err != nil {
			resp.Diagnostics.AddError("Bridge Resource Error",
				"Unable to create a new bridge.")
			resp.Diagnostics.Append(res.Diagnostics()...)
			return
		}

	}

	// Configure an IPv6 address if specified.
	if !data.IPv6Address.IsNull() && !data.IPv6Address.IsUnknown() {
		addr := data.IPv6Address.ValueString()

		// Validate the CIDR format
		_, _, err := net.ParseCIDR(addr)
		if err != nil {
			resp.Diagnostics.AddError("Invalid IPv6 address format",
				fmt.Sprintf("IPv6 address must be in CIDR format (e.g., 'fd00::1/64'): %s", err.Error()))
			return
		}

		moreArgs := []string{"addr", "add", addr, "dev", br}
		res, err := r.providerConf.Exec.Run(ctx, d, ipCmd, append(ipArgs, moreArgs...)...)
		if err != nil {
			resp.Diagnostics.AddError("Bridge Resource Error",
				"Unable to create a new bridge.")
			resp.Diagnostics.Append(res.Diagnostics()...)
			return
		}

	}

	// Enslave any existing interfaces specified, attaching them as members
	// of the bridge and bringing them up so traffic can flow through it.
	if !data.EnslavedInterfaces.IsNull() && !data.EnslavedInterfaces.IsUnknown() {
		var ifaces []string
		resp.Diagnostics.Append(data.EnslavedInterfaces.ElementsAs(ctx, &ifaces, false)...)
		if resp.Diagnostics.HasError() {
			return
		}

		for _, intf := range ifaces {
			// Attach the interface as a member of the bridge.
			moreArgs := []string{"link", "set", "dev", intf, "master", br}
			res, err := r.providerConf.Exec.Run(ctx, d, ipCmd, append(ipArgs, moreArgs...)...)
			if err != nil {
				resp.Diagnostics.AddError("Bridge Resource Error",
					fmt.Sprintf("Unable to enslave interface '%s' to bridge '%s'.", intf, br))
				resp.Diagnostics.Append(res.Diagnostics()...)
				return
			}

			// Bring the enslaved interface up. NOTE: this MUST be done after
			// setting the master.
			moreArgs = []string{"link", "set", "dev", intf, "up"}
			res, err = r.providerConf.Exec.Run(ctx, d, ipCmd, append(ipArgs, moreArgs...)...)
			if err != nil {
				resp.Diagnostics.AddError("Bridge Resource Error",
					fmt.Sprintf("Unable to bring enslaved interface '%s' up.", intf))
				resp.Diagnostics.Append(res.Diagnostics()...)
				return
			}
		}
	}

	// Read the bridge current state.
	if diags, err := r.readBridge(ctx, d, ipCmd, ipArgs, &data); err != nil {
		resp.Diagnostics.AddError("Failed to read bridge state", err.Error())
		resp.Diagnostics.Append(diags...)
		return
	}

	// Now we know the MAC address of the bridge whether it was specifically
	// configured or not.
	if !data.IPv6Address.IsNull() && !data.IPv6Address.IsUnknown() {
		// If the configuration also included an IPv6 address we need
		// to ensure that the bridge interface has a link-local address
		// configured.
		//
		// NOTE: This is usually handled automatically by the Linux
		// kernel however that automatic link-local address config only
		// happens when the interface state changes to UP, which depending
		// on other resources (like VMs starting) might only happen much
		// later. At the same time other resources like RADV depend only
		// the interface having a link-local address sooner.
		ll, err := lladdr.LinkLocalIPv6FromMACString(data.MACAddress.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Bridge Resource Error",
				fmt.Sprintf("Can't configure link-local address '%s' on bridge interface: %v", ll.String(), err))
			return
		}
		moreArgs := []string{"addr", "add", fmt.Sprintf("%s/64", ll.String()), "scope", "link", "dev", br}
		res, err := r.providerConf.Exec.Run(ctx, d, ipCmd, append(ipArgs, moreArgs...)...)
		if err != nil {
			resp.Diagnostics.AddError("Bridge Resource Error",
				"Unable to create a new bridge.")
			resp.Diagnostics.Append(res.Diagnostics()...)
			return
		}
	}

	tflog.Trace(ctx, "Bridge Resource created successfully")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *Bridge) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data BridgeModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d := r.getResourceDir(data.ID.ValueString())

	netns := ""
	if !data.NetNS.IsNull() && !data.NetNS.IsUnknown() {
		netns = data.NetNS.ValueString()
	}
	ipCmd, ipArgs := buildIPCommand(r.providerConf, netns)

	// Read the bridge current state.
	if diags, err := r.readBridge(ctx, d, ipCmd, ipArgs, &data); err != nil {
		// Check for various error messages that indicate the device doesn't exist.
		if errchecker.ContainsAny(err, intfNotFoundStrs) || errchecker.DiagsAny(diags, intfNotFoundStrs) {
			// Resource was deleted outside Terraform: remove from state.
			resp.State.RemoveResource(ctx)
			return
		}

		resp.Diagnostics.AddError("Failed to read bridge state", err.Error())
		resp.Diagnostics.Append(diags...)
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *Bridge) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data BridgeModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// TODO: What to do here ?

	resp.Diagnostics.AddError("Bridge Resource Update Error", "Update is not supported.")
}

func (r *Bridge) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data BridgeModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	br := data.Name.ValueString()
	d := r.getResourceDir(data.ID.ValueString())

	netns := ""
	if !data.NetNS.IsNull() && !data.NetNS.IsUnknown() {
		netns = data.NetNS.ValueString()
	}
	ipCmd, ipArgs := buildIPCommand(r.providerConf, netns)

	// Delete an existing bridge.
	moreArgs := []string{"link", "delete", br, "type", "bridge"}
	res, err := r.providerConf.Exec.Run(ctx, d, ipCmd, append(ipArgs, moreArgs...)...)
	if err != nil {
		errMsg := err.Error()
		// Check for various error messages that indicate the device doesn't exist.
		// If the device doesn't exist, the delete is successful (idempotent),
		// otherwise we need to treat it like an error.
		if !strings.Contains(errMsg, "does not exist") &&
			!strings.Contains(errMsg, "Cannot find device") &&
			!strings.Contains(errMsg, "cannot find device") &&
			!strings.Contains(errMsg, "No such device") &&
			!strings.Contains(errMsg, "no such device") {
			resp.Diagnostics.AddError("Failed to delete bridge", err.Error())
			resp.Diagnostics.Append(res.Diagnostics()...)
			return
		}
	}

	if err := r.providerConf.Exec.Remove(ctx, d); err != nil {
		resp.Diagnostics.AddError("Bridge Resource Delete Error",
			fmt.Sprintf("Can't delete bridge resource directory: %v", err))
		return
	}
}

func (r *Bridge) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *Bridge) readBridge(ctx context.Context, resPath string, ipCmd string, ipArgs []string, model *BridgeModel) (diag.Diagnostics, error) {
	br := model.Name.ValueString()

	// Check if bridge exists and get info.
	moreArgs := []string{"link", "show", br}
	res, err := r.providerConf.Exec.Run(ctx, resPath, ipCmd, append(ipArgs, moreArgs...)...)
	if err != nil {
		return res.Diagnostics(), fmt.Errorf("can't retrieve bridge '%s' details: %w", br, err)
	}

	// Parse output for MTU, state, and MAC address.
	lines := strings.Split(res.Stdout, "\n")
	// A real `ip link show` line always contains the flags section "<...>".
	// If it doesn't, the output is empty or unparseable: fail loudly with the
	// raw output instead of fabricating a "down" state.
	if len(lines) == 0 || !strings.Contains(lines[0], "<") {
		return res.Diagnostics(),
			fmt.Errorf("can't parse bridge '%s' link details, unexpected output: %q", br, res.Stdout)
	}
	// Parse first line: "2: br0: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 ..."
	mtuRegex := regexp.MustCompile(`mtu (\d+)`)
	if matches := mtuRegex.FindStringSubmatch(lines[0]); len(matches) > 1 {
		x, err := strconv.ParseInt(matches[1], 10, 64)
		if err != nil {
			e := fmt.Errorf("invalid bridge '%s' MTU value '%s': %w", br, matches[1], err)
			d := diag.Diagnostics{}
			d.AddError("Can't find bridge interface MTU value", e.Error())
			return d, e
		}
		model.MTU = types.Int64Value(x)
	}

	// Determine state from the administrative UP flag inside "<...>".
	if linkFlagUp(lines[0]) {
		model.State = types.StringValue("up")
	} else {
		model.State = types.StringValue("down")
	}

	// Parse MAC address from second line: "    link/ether 00:11:22:33:44:55 brd ff:ff:ff:ff:ff:ff"
	if len(lines) > 1 {
		macRegex := regexp.MustCompile(`link/ether\s+([0-9a-fA-F:]+)`)
		if matches := macRegex.FindStringSubmatch(lines[1]); len(matches) > 1 {
			model.MACAddress = types.StringValue(matches[1])
		}
	}

	// Get IP address(es) of bridge.
	moreArgs = []string{"addr", "show", br}
	res, err = r.providerConf.Exec.Run(ctx, resPath, ipCmd, append(ipArgs, moreArgs...)...)
	if err != nil {
		return res.Diagnostics(), fmt.Errorf("can't retrieve bridge '%s' addreses: %w", br, err)
	}
	// Look for IPv4 address in CIDR format: inet 192.168.1.1/24 brd ...
	addrRegex := regexp.MustCompile(`inet (\d+\.\d+\.\d+\.\d+/\d+)`)
	if matches := addrRegex.FindStringSubmatch(res.Stdout); len(matches) > 1 {
		// Validate using net.ParseCIDR to ensure it's properly formatted
		if _, _, err := net.ParseCIDR(matches[1]); err == nil {
			model.IPv4Address = types.StringValue(matches[1])
		}
	}

	// Look for IPv6 address in CIDR format: inet6 fd00::1/64 scope ...
	// Skip link-local addresses (fe80::/10) as these are auto-configured
	addr6Regex := regexp.MustCompile(`inet6 ([0-9a-fA-F:]+/\d+)`)
	matches := addr6Regex.FindAllStringSubmatch(res.Stdout, -1)
	for _, match := range matches {
		if len(match) > 1 {
			// Parse and validate the address
			ip, _, err := net.ParseCIDR(match[1])
			if err != nil {
				continue
			}
			// Skip link-local addresses (fe80::/10)
			if !ip.IsLinkLocalUnicast() {
				model.IPv6Address = types.StringValue(match[1])
				break
			}
		}
	}

	// Reconcile the set of interfaces this resource explicitly enslaved.
	//
	// `ip link show master <br>` lists EVERY interface whose master is this
	// bridge, but other resources may attach members on their own (e.g. a
	// zedamigo_tap with its own `master` set). Those foreign members must not
	// land in `enslaved_interfaces`: this attribute is RequiresReplace, so
	// reporting them would force a spurious destroy/recreate of the bridge.
	//
	// Instead we reconcile only against the interfaces already recorded in the
	// incoming model (prior state on Read, the configured set on Create): keep
	// the ones we manage that are still members (so real drift is detected),
	// drop everything else, and stay null when we manage none.
	if model.EnslavedInterfaces.IsNull() || model.EnslavedInterfaces.IsUnknown() {
		model.EnslavedInterfaces = types.SetNull(types.StringType)
	} else {
		moreArgs = []string{"link", "show", "master", br}
		res, err = r.providerConf.Exec.Run(ctx, resPath, ipCmd, append(ipArgs, moreArgs...)...)
		if err != nil {
			return res.Diagnostics(), fmt.Errorf("can't retrieve bridge '%s' members: %w", br, err)
		}
		// Capture the interface name from each index line, stopping at ':'
		// (plain names) or '@' (e.g. VLAN sub-interfaces like "eth1.10@eth1").
		memberRegex := regexp.MustCompile(`(?m)^\s*\d+:\s+([^:@\s]+)`)
		current := make(map[string]bool)
		for _, m := range memberRegex.FindAllStringSubmatch(res.Stdout, -1) {
			if len(m) > 1 {
				current[m[1]] = true
			}
		}

		// Keep only managed interfaces that are still members of the bridge.
		kept := make([]attr.Value, 0, len(model.EnslavedInterfaces.Elements()))
		for _, e := range model.EnslavedInterfaces.Elements() {
			s, ok := e.(types.String)
			if !ok || s.IsNull() || s.IsUnknown() {
				continue
			}
			if current[s.ValueString()] {
				kept = append(kept, s)
			}
		}
		if len(kept) > 0 {
			members, diags := types.SetValue(types.StringType, kept)
			if diags.HasError() {
				return diags, fmt.Errorf("can't build bridge '%s' members set", br)
			}
			model.EnslavedInterfaces = members
		} else {
			// Nothing we manage remains: keep null to match an unconfigured plan.
			model.EnslavedInterfaces = types.SetNull(types.StringType)
		}
	}

	return nil, nil
}
