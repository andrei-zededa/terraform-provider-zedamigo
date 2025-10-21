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
	vlanIntfsDir = "vlan_sub_intfs"
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                = &VLAN{}
	_ resource.ResourceWithImportState = &VLAN{}
)

func NewVLAN() resource.Resource {
	return &VLAN{}
}

// VLAN defines the resource implementation.
type VLAN struct {
	providerConf *ZedAmigoProviderConfig
}

// VLANModel describes the resource data model.
type VLANModel struct {
	ID          types.String `tfsdk:"id"`
	VlanID      types.Int64  `tfsdk:"vlan_id"`
	Name        types.String `tfsdk:"name"`
	MTU         types.Int64  `tfsdk:"mtu"`
	State       types.String `tfsdk:"state"`
	Parent      types.String `tfsdk:"parent"`
	IPv4Address types.String `tfsdk:"ipv4_address"`
}

func (r *VLAN) getResourceDir(id string) string {
	return filepath.Join(r.providerConf.LibPath, vlanIntfsDir, id)
}

func (r *VLAN) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vlan"
}

func (r *VLAN) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description:         "VLAN sub-interface",
		MarkdownDescription: "Create and manage a Linux network VLAN sub-interface on a specific parent interface. This is done using iproute2 commands.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				Description:         "VLAN resource identifier",
				MarkdownDescription: "VLAN resource identifier. This is not the VLAN ID, since we can have multiple sub-interfaces with the same VLAN ID on different parent interfaces.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"vlan_id": schema.Int64Attribute{
				Description: "VLAN ID for this sub-interface",
				Required:    true,
			},
			"name": schema.StringAttribute{
				Computed:    true,
				Description: "Full name of the resuling interface (e.g. `${parent}.${vlanid}`).",
			},
			"mtu": schema.Int64Attribute{
				Description: "MTU size for the VLAN sub-interface",
				Optional:    true,
				Computed:    true,
			},
			"state": schema.StringAttribute{
				Description: "State of the VLAN sub-interface (up/down)",
				Optional:    true,
				Computed:    true,
			},
			"parent": schema.StringAttribute{
				Description: "Parent interface on which to create this VLAN sub-interface",
				Optional:    false,
				Required:    true,
			},
			"ipv4_address": schema.StringAttribute{
				Description: "IPv4 address to configure on the VLAN sub-interface",
				Optional:    true,
			},
		},
	}
}

func (r *VLAN) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
	tflog.Trace(ctx, "VLAN resource configure debugging", traceData)
}

func (r *VLAN) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data VLANModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	u, err := uuid.NewV4()
	if err != nil {
		resp.Diagnostics.AddError("VLAN Resource Error",
			fmt.Sprintf("Unable to generate a new UUID: %s", err))
		return
	}
	data.ID = types.StringValue(u.String())
	if resp.Diagnostics.HasError() {
		return
	}

	d := r.getResourceDir(data.ID.ValueString())
	if err := os.MkdirAll(d, 0o700); err != nil {
		resp.Diagnostics.AddError("VLAN Resource Error",
			fmt.Sprintf("Unable to create resource specific directory: %s", err))
		return
	}
	if err := createTFBackPointer(d); err != nil {
		resp.Diagnostics.AddError("VLAN Resource Error",
			fmt.Sprintf("Unable to create resource specific file: %s", err))
		return
	}

	subIf := fmt.Sprintf("%s.%d", data.Parent.ValueString(), data.VlanID.ValueInt64())
	data.Name = types.StringValue(subIf)

	ipCmd := r.providerConf.IP
	ipArgs := []string{}
	if r.providerConf.UseSudo {
		ipCmd = r.providerConf.Sudo
		ipArgs = []string{"-n", r.providerConf.IP}
	}

	// Create the VLAN sub-interface, e.g. sudo ip link add link eth100 name eth100.64 type vlan id 64
	moreArgs := []string{
		"link", "add", "link", data.Parent.ValueString(), "name", subIf,
		"type", "vlan", "id", fmt.Sprintf("%d", data.VlanID.ValueInt64()),
	}
	res, err := cmd.Run(d, ipCmd, append(ipArgs, moreArgs...)...)
	if err != nil {
		resp.Diagnostics.AddError("VLAN Resource Error",
			"Unable to create a new VLAN.")
		resp.Diagnostics.Append(res.Diagnostics()...)
		return
	}

	// Set the MTU if specified.
	if !data.MTU.IsNull() && !data.MTU.IsUnknown() {
		mtu := fmt.Sprintf("%d", data.MTU.ValueInt64())
		moreArgs := []string{"link", "set", "dev", subIf, "mtu", mtu}
		res, err := cmd.Run(d, ipCmd, append(ipArgs, moreArgs...)...)
		if err != nil {
			resp.Diagnostics.AddError("VLAN Resource Error",
				"Unable to create a new VLAN.")
			resp.Diagnostics.Append(res.Diagnostics()...)
			return
		}
	}

	// Set the state if specified.
	if !data.State.IsNull() && !data.State.IsUnknown() {
		state := data.State.ValueString()
		moreArgs := []string{"link", "set", "dev", subIf, state}
		res, err := cmd.Run(d, ipCmd, append(ipArgs, moreArgs...)...)
		if err != nil {
			resp.Diagnostics.AddError("VLAN Resource Error",
				"Unable to create a new VLAN.")
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

		moreArgs := []string{"addr", "add", addr, "dev", subIf}
		res, err := cmd.Run(d, ipCmd, append(ipArgs, moreArgs...)...)
		if err != nil {
			resp.Diagnostics.AddError("VLAN Resource Error",
				"Unable to create a new VLAN.")
			resp.Diagnostics.Append(res.Diagnostics()...)
			return
		}

	}

	// Read the VLAN current state.
	if diags, err := r.readVLAN(d, ipCmd, ipArgs, &data); err != nil {
		resp.Diagnostics.AddError("Failed to read VLAN state", err.Error())
		resp.Diagnostics.Append(diags...)
		return
	}

	tflog.Trace(ctx, "VLAN Resource created succesfully")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *VLAN) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data VLANModel

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

	// Read the VLAN current state.
	if diags, err := r.readVLAN(d, ipCmd, ipArgs, &data); err != nil {
		// Check for various error messages that indicate the device doesn't exist.
		if errchecker.ContainsAny(err, intfNotFoundStrs) || errchecker.DiagsAny(diags, intfNotFoundStrs) {
			// Resource was deleted outside Terraform: remove from state.
			resp.State.RemoveResource(ctx)
			return
		}

		resp.Diagnostics.AddError("Failed to read VLAN state", err.Error())
		resp.Diagnostics.Append(diags...)
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *VLAN) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data VLANModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// TODO: What to do here ?

	resp.Diagnostics.AddError("VLAN Resource Update Error", "Update is not supported.")
}

func (r *VLAN) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data VLANModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	subIf := data.Name.ValueString()
	d := r.getResourceDir(data.ID.ValueString())

	ipCmd := r.providerConf.IP
	ipArgs := []string{}
	if r.providerConf.UseSudo {
		ipCmd = r.providerConf.Sudo
		ipArgs = []string{"-n", r.providerConf.IP}
	}

	// Delete an existing VLAN.
	moreArgs := []string{"link", "del", subIf}
	res, err := cmd.Run(d, ipCmd, append(ipArgs, moreArgs...)...)
	if err != nil {
		// Check for various error messages that indicate the device doesn't exist.
		// If the device doesn't exist, the delete is successful (idempotent),
		// otherwise we need to treat it like an error.
		if errchecker.ContainsNone(err, intfNotFoundStrs) &&
			errchecker.DiagsNone(res.Diagnostics(), intfNotFoundStrs) {
			resp.Diagnostics.AddError("Failed to delete VLAN", err.Error())
			resp.Diagnostics.Append(res.Diagnostics()...)
			return
		}
	}

	if err := os.RemoveAll(d); err != nil {
		resp.Diagnostics.AddError("VLAN Resource Delete Error",
			fmt.Sprintf("Can't delete VLAN resource directory: %v", err))
		return
	}
}

func (r *VLAN) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *VLAN) readVLAN(resPath string, ipCmd string, ipArgs []string, model *VLANModel) (diag.Diagnostics, error) {
	subIf := model.Name.ValueString()

	// Check if VLAN exists and get info.
	moreArgs := []string{"link", "show", subIf}
	res, err := cmd.Run(resPath, ipCmd, append(ipArgs, moreArgs...)...)
	if err != nil {
		return res.Diagnostics(), fmt.Errorf("can't retrieve VLAN '%s' details: %w", subIf, err)
	}

	// Parse output for MTU and state.
	lines := strings.Split(res.Stdout, "\n")
	if len(lines) > 0 {
		// Parse first line: "2: br0: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 ..."
		mtuRegex := regexp.MustCompile(`mtu (\d+)`)
		if matches := mtuRegex.FindStringSubmatch(lines[0]); len(matches) > 1 {
			x, err := strconv.ParseInt(matches[1], 10, 64)
			if err != nil {
				e := fmt.Errorf("invalid VLAN '%s' MTU value '%s': %w", subIf, matches[1], err)
				d := diag.Diagnostics{}
				d.AddError("Can't find VLAN interface MTU value", e.Error())
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

	// Get IP address(es) of VLAN.
	moreArgs = []string{"addr", "show", subIf}
	res, err = cmd.Run(resPath, ipCmd, append(ipArgs, moreArgs...)...)
	if err != nil {
		return res.Diagnostics(), fmt.Errorf("can't retrieve VLAN '%s' addreses: %w", subIf, err)
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
