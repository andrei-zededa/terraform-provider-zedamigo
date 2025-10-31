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
	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/lladdr"
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

	u, err := uuid.NewV4()
	if err != nil {
		resp.Diagnostics.AddError("Bridge Resource Error",
			fmt.Sprintf("Unable to generate a new UUID: %s", err))
		return
	}
	data.ID = types.StringValue(u.String())
	if resp.Diagnostics.HasError() {
		return
	}

	d := r.getResourceDir(data.ID.ValueString())
	if err := os.MkdirAll(d, 0o700); err != nil {
		resp.Diagnostics.AddError("Bridge Resource Error",
			fmt.Sprintf("Unable to create resource specific directory: %s", err))
		return
	}
	if err := createTFBackPointer(d); err != nil {
		resp.Diagnostics.AddError("Bridge Resource Error",
			fmt.Sprintf("Unable to create resource specific file: %s", err))
		return
	}

	br := data.Name.ValueString()

	ipCmd := r.providerConf.IP
	ipArgs := []string{}
	if r.providerConf.UseSudo {
		ipCmd = r.providerConf.Sudo
		ipArgs = []string{"-n", r.providerConf.IP}
	}

	// Create the bridge.
	moreArgs := []string{"link", "add", br, "type", "bridge"}
	res, err := cmd.Run(d, ipCmd, append(ipArgs, moreArgs...)...)
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
		res, err := cmd.Run(d, ipCmd, append(ipArgs, moreArgs...)...)
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
		res, err := cmd.Run(d, ipCmd, append(ipArgs, moreArgs...)...)
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
		res, err := cmd.Run(d, ipCmd, append(ipArgs, moreArgs...)...)
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
		res, err := cmd.Run(d, ipCmd, append(ipArgs, moreArgs...)...)
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
		res, err := cmd.Run(d, ipCmd, append(ipArgs, moreArgs...)...)
		if err != nil {
			resp.Diagnostics.AddError("Bridge Resource Error",
				"Unable to create a new bridge.")
			resp.Diagnostics.Append(res.Diagnostics()...)
			return
		}

	}

	// Read the bridge current state.
	if diags, err := r.readBridge(d, ipCmd, ipArgs, &data); err != nil {
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
		res, err := cmd.Run(d, ipCmd, append(ipArgs, moreArgs...)...)
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

	ipCmd := r.providerConf.IP
	ipArgs := []string{}
	if r.providerConf.UseSudo {
		ipCmd = r.providerConf.Sudo
		ipArgs = []string{"-n", r.providerConf.IP}
	}

	// Read the bridge current state.
	if diags, err := r.readBridge(d, ipCmd, ipArgs, &data); err != nil {
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

	ipCmd := r.providerConf.IP
	ipArgs := []string{}
	if r.providerConf.UseSudo {
		ipCmd = r.providerConf.Sudo
		ipArgs = []string{"-n", r.providerConf.IP}
	}

	// Delete an existing bridge.
	moreArgs := []string{"link", "delete", br, "type", "bridge"}
	res, err := cmd.Run(d, ipCmd, append(ipArgs, moreArgs...)...)
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

	if err := os.RemoveAll(d); err != nil {
		resp.Diagnostics.AddError("Bridge Resource Delete Error",
			fmt.Sprintf("Can't delete bridge resource directory: %v", err))
		return
	}
}

func (r *Bridge) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *Bridge) readBridge(resPath string, ipCmd string, ipArgs []string, model *BridgeModel) (diag.Diagnostics, error) {
	br := model.Name.ValueString()

	// Check if bridge exists and get info.
	moreArgs := []string{"link", "show", br}
	res, err := cmd.Run(resPath, ipCmd, append(ipArgs, moreArgs...)...)
	if err != nil {
		return res.Diagnostics(), fmt.Errorf("can't retrieve bridge '%s' details: %w", br, err)
	}

	// Parse output for MTU, state, and MAC address.
	lines := strings.Split(res.Stdout, "\n")
	if len(lines) > 0 {
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

		if strings.Contains(lines[0], "UP") {
			model.State = types.StringValue("up")
		} else {
			model.State = types.StringValue("down")
		}
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
	res, err = cmd.Run(resPath, ipCmd, append(ipArgs, moreArgs...)...)
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

	return nil, nil
}
