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
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

const (
	lagIntfsDir = "lag_intfs"
)

// addrNotFoundStrs are substrings present in `ip addr del` errors when the
// target address is not (or no longer) configured on the interface. Used to
// make address reconciliation idempotent.
var addrNotFoundStrs = []string{
	"Cannot assign requested address",
	"cannot assign requested address",
}

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                   = &LAG{}
	_ resource.ResourceWithImportState    = &LAG{}
	_ resource.ResourceWithValidateConfig = &LAG{}
)

func NewLAG() resource.Resource {
	return &LAG{}
}

// LAG defines the resource implementation.
type LAG struct {
	providerConf *ZedAmigoProviderConfig
}

// LAGModel describes the resource data model.
type LAGModel struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	Mode           types.String `tfsdk:"mode"`
	MIIMon         types.Int64  `tfsdk:"miimon"`
	LACPRate       types.String `tfsdk:"lacp_rate"`
	XmitHashPolicy types.String `tfsdk:"xmit_hash_policy"`
	MTU            types.Int64  `tfsdk:"mtu"`
	State          types.String `tfsdk:"state"`
	MACAddress     types.String `tfsdk:"mac_address"`
	IPv4Address    types.String `tfsdk:"ipv4_address"`
	IPv6Address    types.String `tfsdk:"ipv6_address"`
	NetNS          types.String `tfsdk:"netns"`
	// EnslavedInterfaces is the set of existing interfaces to aggregate as
	// members (slaves) of this bond. Unlike zedamigo_bridge, changing this set
	// is applied in-place (members are added/removed without re-creating the
	// bond).
	EnslavedInterfaces types.Set `tfsdk:"enslaved_interfaces"`
}

func (r *LAG) getResourceDir(id string) string {
	return filepath.Join(r.providerConf.LibPath, lagIntfsDir, id)
}

func (r *LAG) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_lag"
}

func (r *LAG) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "LAG (Linux bond)",
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Create and manage a Linux bonding (link-aggregation / LAG) interface using iproute2 commands.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				Description:         "LAG identifier",
				MarkdownDescription: "LAG identifier",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Name of the bond interface",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"mode": schema.StringAttribute{
				Description: "Bonding mode: balance-rr, active-backup, balance-xor, broadcast, " +
					"802.3ad, balance-tlb or balance-alb. If omitted the kernel default " +
					"(balance-rr) is used. Changing it re-creates the bond.",
				Optional: true,
				Validators: []validator.String{
					stringvalidator.OneOf("balance-rr", "active-backup", "balance-xor",
						"broadcast", "802.3ad", "balance-tlb", "balance-alb"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"miimon": schema.Int64Attribute{
				Description: "MII link monitoring interval in milliseconds (0 disables it). " +
					"Changing it re-creates the bond.",
				Optional: true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"lacp_rate": schema.StringAttribute{
				Description: "LACPDU transmit rate for 802.3ad mode: slow or fast. " +
					"Only valid when mode is 802.3ad. Changing it re-creates the bond.",
				Optional: true,
				Validators: []validator.String{
					stringvalidator.OneOf("slow", "fast"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"xmit_hash_policy": schema.StringAttribute{
				Description: "Transmit hash policy for 802.3ad and balance-xor modes: " +
					"layer2, layer2+3, layer3+4, encap2+3, encap3+4 or vlan+srcmac. " +
					"Changing it re-creates the bond.",
				Optional: true,
				Validators: []validator.String{
					stringvalidator.OneOf("layer2", "layer2+3", "layer3+4",
						"encap2+3", "encap3+4", "vlan+srcmac"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"mtu": schema.Int64Attribute{
				Description: "MTU size for the bond",
				Optional:    true,
				Computed:    true,
			},
			"state": schema.StringAttribute{
				Description: "State of the bond (up/down)",
				Optional:    true,
				Computed:    true,
			},
			"mac_address": schema.StringAttribute{
				Description: "MAC address for the bond. If left unset the bond adopts the MAC " +
					"of the first enslaved member; set it explicitly for a stable address.",
				Optional: true,
				Computed: true,
			},
			"ipv4_address": schema.StringAttribute{
				Description: "IPv4 address for the bond",
				Optional:    true,
			},
			"ipv6_address": schema.StringAttribute{
				Description: "IPv6 address for the bond",
				Optional:    true,
			},
			"netns": schema.StringAttribute{
				Description: "Network namespace in which to create the bond",
				Optional:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"enslaved_interfaces": schema.SetAttribute{
				Description: "Names of existing interfaces to aggregate as members (slaves) of this bond. " +
					"Each interface is brought down, attached with `ip link set dev <interface> master <bond>` " +
					"and brought back up. The interfaces must already exist in the same network namespace as " +
					"the bond. Changing this set is applied in-place (members are added/removed without " +
					"re-creating the bond). " +
					"Only list interfaces that are not otherwise managed as bond members: do not include " +
					"interfaces (such as `zedamigo_tap` resources) that attach themselves via their own `master` " +
					"attribute, as those are owned by the other resource and are deliberately ignored here.",
				Optional:    true,
				ElementType: types.StringType,
			},
		},
	}
}

// ValidateConfig rejects combinations the bonding driver would refuse, with a
// clearer message than the raw iproute2 error.
func (r *LAG) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data LAGModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Mode may be unknown (e.g. referenced from another resource); skip the
	// cross-attribute checks in that case.
	if data.Mode.IsUnknown() {
		return
	}
	mode := ""
	if !data.Mode.IsNull() {
		mode = data.Mode.ValueString()
	}

	if !data.LACPRate.IsNull() && !data.LACPRate.IsUnknown() && mode != "802.3ad" {
		resp.Diagnostics.AddAttributeError(
			path.Root("lacp_rate"),
			"Invalid lacp_rate usage",
			`"lacp_rate" is only valid when "mode" is "802.3ad".`,
		)
	}

	if !data.XmitHashPolicy.IsNull() && !data.XmitHashPolicy.IsUnknown() &&
		mode != "802.3ad" && mode != "balance-xor" {
		resp.Diagnostics.AddAttributeError(
			path.Root("xmit_hash_policy"),
			"Invalid xmit_hash_policy usage",
			`"xmit_hash_policy" is only valid when "mode" is "802.3ad" or "balance-xor".`,
		)
	}
}

func (r *LAG) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
	tflog.Trace(ctx, "LAG resource configure debugging", traceData)
}

func (r *LAG) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data LAGModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, err := newResourceID()
	if err != nil {
		resp.Diagnostics.AddError("LAG Resource Error",
			fmt.Sprintf("Unable to generate a new resource ID: %s", err))
		return
	}
	data.ID = types.StringValue(id)

	d := r.getResourceDir(data.ID.ValueString())
	if err := os.MkdirAll(d, 0o700); err != nil {
		resp.Diagnostics.AddError("LAG Resource Error",
			fmt.Sprintf("Unable to create resource specific directory: %s", err))
		return
	}
	if err := createTFBackPointer(d); err != nil {
		resp.Diagnostics.AddError("LAG Resource Error",
			fmt.Sprintf("Unable to create resource specific file: %s", err))
		return
	}

	bond := data.Name.ValueString()

	netns := ""
	if !data.NetNS.IsNull() && !data.NetNS.IsUnknown() {
		netns = data.NetNS.ValueString()
	}
	ipCmd, ipArgs := buildIPCommand(r.providerConf, netns)

	// Create the bond with its (immutable) bonding options.
	moreArgs := []string{"link", "add", bond, "type", "bond"}
	if !data.Mode.IsNull() && !data.Mode.IsUnknown() {
		moreArgs = append(moreArgs, "mode", data.Mode.ValueString())
	}
	if !data.MIIMon.IsNull() && !data.MIIMon.IsUnknown() {
		moreArgs = append(moreArgs, "miimon", fmt.Sprintf("%d", data.MIIMon.ValueInt64()))
	}
	if !data.LACPRate.IsNull() && !data.LACPRate.IsUnknown() {
		moreArgs = append(moreArgs, "lacp_rate", data.LACPRate.ValueString())
	}
	if !data.XmitHashPolicy.IsNull() && !data.XmitHashPolicy.IsUnknown() {
		moreArgs = append(moreArgs, "xmit_hash_policy", data.XmitHashPolicy.ValueString())
	}
	res, err := cmd.Run(d, ipCmd, append(ipArgs, moreArgs...)...)
	if err != nil {
		resp.Diagnostics.AddError("LAG Resource Error",
			"Unable to create a new bond.")
		resp.Diagnostics.Append(res.Diagnostics()...)
		return
	}

	// Set the MTU if specified (propagates to members as they are enslaved).
	if !data.MTU.IsNull() && !data.MTU.IsUnknown() {
		mtu := fmt.Sprintf("%d", data.MTU.ValueInt64())
		moreArgs := []string{"link", "set", "dev", bond, "mtu", mtu}
		res, err := cmd.Run(d, ipCmd, append(ipArgs, moreArgs...)...)
		if err != nil {
			resp.Diagnostics.AddError("LAG Resource Error",
				"Unable to set bond MTU.")
			resp.Diagnostics.Append(res.Diagnostics()...)
			return
		}
	}

	// Set the MAC address if specified. Doing this before enslaving members
	// means the explicit address wins over the first member's address.
	if !data.MACAddress.IsNull() && !data.MACAddress.IsUnknown() {
		macAddr := data.MACAddress.ValueString()
		moreArgs := []string{"link", "set", "dev", bond, "address", macAddr}
		res, err := cmd.Run(d, ipCmd, append(ipArgs, moreArgs...)...)
		if err != nil {
			resp.Diagnostics.AddError("LAG Resource Error",
				"Unable to set MAC address for bond.")
			resp.Diagnostics.Append(res.Diagnostics()...)
			return
		}
	}

	// Enslave any specified members, attaching them to the bond.
	if !data.EnslavedInterfaces.IsNull() && !data.EnslavedInterfaces.IsUnknown() {
		var ifaces []string
		resp.Diagnostics.Append(data.EnslavedInterfaces.ElementsAs(ctx, &ifaces, false)...)
		if resp.Diagnostics.HasError() {
			return
		}

		for _, intf := range ifaces {
			if diags, err := r.enslaveMember(d, ipCmd, ipArgs, bond, intf); err != nil {
				resp.Diagnostics.AddError("LAG Resource Error", err.Error())
				resp.Diagnostics.Append(diags...)
				return
			}
		}
	}

	// Configure an IPv4 address if specified.
	if !data.IPv4Address.IsNull() && !data.IPv4Address.IsUnknown() {
		addr := data.IPv4Address.ValueString()

		// Validate the CIDR format
		if _, _, err := net.ParseCIDR(addr); err != nil {
			resp.Diagnostics.AddError("Invalid IPv4 address format",
				fmt.Sprintf("IPv4 address must be in CIDR format (e.g., '192.168.1.1/24'): %s", err.Error()))
			return
		}

		moreArgs := []string{"addr", "add", addr, "dev", bond}
		res, err := cmd.Run(d, ipCmd, append(ipArgs, moreArgs...)...)
		if err != nil {
			resp.Diagnostics.AddError("LAG Resource Error",
				"Unable to configure bond IPv4 address.")
			resp.Diagnostics.Append(res.Diagnostics()...)
			return
		}
	}

	// Configure an IPv6 address if specified.
	if !data.IPv6Address.IsNull() && !data.IPv6Address.IsUnknown() {
		addr := data.IPv6Address.ValueString()

		// Validate the CIDR format
		if _, _, err := net.ParseCIDR(addr); err != nil {
			resp.Diagnostics.AddError("Invalid IPv6 address format",
				fmt.Sprintf("IPv6 address must be in CIDR format (e.g., 'fd00::1/64'): %s", err.Error()))
			return
		}

		moreArgs := []string{"addr", "add", addr, "dev", bond}
		res, err := cmd.Run(d, ipCmd, append(ipArgs, moreArgs...)...)
		if err != nil {
			resp.Diagnostics.AddError("LAG Resource Error",
				"Unable to configure bond IPv6 address.")
			resp.Diagnostics.Append(res.Diagnostics()...)
			return
		}
	}

	// Set the state if specified. NOTE: this is done after enslaving members so
	// that bringing the bond up carries its members.
	if !data.State.IsNull() && !data.State.IsUnknown() {
		state := data.State.ValueString()
		moreArgs := []string{"link", "set", "dev", bond, state}
		res, err := cmd.Run(d, ipCmd, append(ipArgs, moreArgs...)...)
		if err != nil {
			resp.Diagnostics.AddError("LAG Resource Error",
				"Unable to set bond state.")
			resp.Diagnostics.Append(res.Diagnostics()...)
			return
		}
	}

	// Read the bond current state.
	if diags, err := r.readLAG(d, ipCmd, ipArgs, &data); err != nil {
		resp.Diagnostics.AddError("Failed to read bond state", err.Error())
		resp.Diagnostics.Append(diags...)
		return
	}

	// Now we know the MAC address of the bond whether it was specifically
	// configured or not. If an IPv6 address is configured, ensure the bond has
	// a link-local address (see the matching note in bridge_resource.go).
	if !data.IPv6Address.IsNull() && !data.IPv6Address.IsUnknown() {
		ll, err := lladdr.LinkLocalIPv6FromMACString(data.MACAddress.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("LAG Resource Error",
				fmt.Sprintf("Can't configure link-local address '%s' on bond interface: %v", ll.String(), err))
			return
		}
		moreArgs := []string{"addr", "add", fmt.Sprintf("%s/64", ll.String()), "scope", "link", "dev", bond}
		res, err := cmd.Run(d, ipCmd, append(ipArgs, moreArgs...)...)
		if err != nil {
			resp.Diagnostics.AddError("LAG Resource Error",
				"Unable to configure bond link-local address.")
			resp.Diagnostics.Append(res.Diagnostics()...)
			return
		}
	}

	tflog.Trace(ctx, "LAG Resource created successfully")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *LAG) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data LAGModel

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

	// Read the bond current state.
	if diags, err := r.readLAG(d, ipCmd, ipArgs, &data); err != nil {
		// Check for various error messages that indicate the device doesn't exist.
		if errchecker.ContainsAny(err, intfNotFoundStrs) || errchecker.DiagsAny(diags, intfNotFoundStrs) {
			// Resource was deleted outside Terraform: remove from state.
			resp.State.RemoveResource(ctx)
			return
		}

		resp.Diagnostics.AddError("Failed to read bond state", err.Error())
		resp.Diagnostics.Append(diags...)
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *LAG) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan LAGModel
	var state LAGModel

	// Read Terraform plan and current state.
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	bond := plan.Name.ValueString()
	d := r.getResourceDir(state.ID.ValueString())

	// name/netns/mode/miimon/lacp_rate/xmit_hash_policy are RequiresReplace, so
	// they cannot reach Update changed: only the in-place attributes below can.
	netns := ""
	if !plan.NetNS.IsNull() && !plan.NetNS.IsUnknown() {
		netns = plan.NetNS.ValueString()
	}
	ipCmd, ipArgs := buildIPCommand(r.providerConf, netns)

	// MTU.
	if !plan.MTU.Equal(state.MTU) && !plan.MTU.IsNull() && !plan.MTU.IsUnknown() {
		mtu := fmt.Sprintf("%d", plan.MTU.ValueInt64())
		moreArgs := []string{"link", "set", "dev", bond, "mtu", mtu}
		res, err := cmd.Run(d, ipCmd, append(ipArgs, moreArgs...)...)
		if err != nil {
			resp.Diagnostics.AddError("LAG Resource Update Error", "Unable to set bond MTU.")
			resp.Diagnostics.Append(res.Diagnostics()...)
			return
		}
	}

	// MAC address.
	if !plan.MACAddress.Equal(state.MACAddress) && !plan.MACAddress.IsNull() && !plan.MACAddress.IsUnknown() {
		moreArgs := []string{"link", "set", "dev", bond, "address", plan.MACAddress.ValueString()}
		res, err := cmd.Run(d, ipCmd, append(ipArgs, moreArgs...)...)
		if err != nil {
			resp.Diagnostics.AddError("LAG Resource Update Error", "Unable to set bond MAC address.")
			resp.Diagnostics.Append(res.Diagnostics()...)
			return
		}
	}

	// IPv4 / IPv6 addresses (del old, add new).
	if !plan.IPv4Address.Equal(state.IPv4Address) {
		if diags, err := r.reconcileAddr(d, ipCmd, ipArgs, bond, state.IPv4Address, plan.IPv4Address); err != nil {
			resp.Diagnostics.AddError("LAG Resource Update Error", err.Error())
			resp.Diagnostics.Append(diags...)
			return
		}
	}
	if !plan.IPv6Address.Equal(state.IPv6Address) {
		if diags, err := r.reconcileAddr(d, ipCmd, ipArgs, bond, state.IPv6Address, plan.IPv6Address); err != nil {
			resp.Diagnostics.AddError("LAG Resource Update Error", err.Error())
			resp.Diagnostics.Append(diags...)
			return
		}
	}

	// Members: add/remove the delta in place.
	if !plan.EnslavedInterfaces.IsUnknown() {
		planMembers, d1 := lagMembersSlice(ctx, plan.EnslavedInterfaces)
		stateMembers, d2 := lagMembersSlice(ctx, state.EnslavedInterfaces)
		resp.Diagnostics.Append(d1...)
		resp.Diagnostics.Append(d2...)
		if resp.Diagnostics.HasError() {
			return
		}

		// Release members we no longer manage first, then enslave new ones, to
		// avoid transient "already has a master" errors when an interface moves.
		for _, intf := range stringsDifference(stateMembers, planMembers) {
			if diags, err := r.releaseMember(d, ipCmd, ipArgs, intf); err != nil {
				resp.Diagnostics.AddError("LAG Resource Update Error", err.Error())
				resp.Diagnostics.Append(diags...)
				return
			}
		}
		for _, intf := range stringsDifference(planMembers, stateMembers) {
			if diags, err := r.enslaveMember(d, ipCmd, ipArgs, bond, intf); err != nil {
				resp.Diagnostics.AddError("LAG Resource Update Error", err.Error())
				resp.Diagnostics.Append(diags...)
				return
			}
		}
	}

	// State last, after members are attached.
	if !plan.State.Equal(state.State) && !plan.State.IsNull() && !plan.State.IsUnknown() {
		moreArgs := []string{"link", "set", "dev", bond, plan.State.ValueString()}
		res, err := cmd.Run(d, ipCmd, append(ipArgs, moreArgs...)...)
		if err != nil {
			resp.Diagnostics.AddError("LAG Resource Update Error", "Unable to set bond state.")
			resp.Diagnostics.Append(res.Diagnostics()...)
			return
		}
	}

	// Read back the current state so all Computed attributes (and the reconciled
	// member set) are concrete before we save.
	if diags, err := r.readLAG(d, ipCmd, ipArgs, &plan); err != nil {
		resp.Diagnostics.AddError("Failed to read bond state after update", err.Error())
		resp.Diagnostics.Append(diags...)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *LAG) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data LAGModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	bond := data.Name.ValueString()
	d := r.getResourceDir(data.ID.ValueString())

	netns := ""
	if !data.NetNS.IsNull() && !data.NetNS.IsUnknown() {
		netns = data.NetNS.ValueString()
	}
	ipCmd, ipArgs := buildIPCommand(r.providerConf, netns)

	// Delete the bond. Deleting the master automatically releases its members,
	// so there is no need to `nomaster` them first.
	moreArgs := []string{"link", "delete", bond, "type", "bond"}
	res, err := cmd.Run(d, ipCmd, append(ipArgs, moreArgs...)...)
	if err != nil {
		// If the device doesn't exist, the delete is successful (idempotent).
		if errchecker.ContainsNone(err, intfNotFoundStrs) &&
			errchecker.DiagsNone(res.Diagnostics(), intfNotFoundStrs) {
			resp.Diagnostics.AddError("Failed to delete bond", err.Error())
			resp.Diagnostics.Append(res.Diagnostics()...)
			return
		}
	}

	if err := os.RemoveAll(d); err != nil {
		resp.Diagnostics.AddError("LAG Resource Delete Error",
			fmt.Sprintf("Can't delete LAG resource directory: %v", err))
		return
	}
}

func (r *LAG) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// enslaveMember attaches intf as a member of bond. Bond slaves must be
// administratively DOWN at the moment they are enslaved, so we force them down
// first, then bring them back up.
func (r *LAG) enslaveMember(resPath string, ipCmd string, ipArgs []string, bond, intf string) (diag.Diagnostics, error) {
	moreArgs := []string{"link", "set", "dev", intf, "down"}
	if res, err := cmd.Run(resPath, ipCmd, append(ipArgs, moreArgs...)...); err != nil {
		return res.Diagnostics(),
			fmt.Errorf("can't set member '%s' down before enslaving to bond '%s': %w", intf, bond, err)
	}

	moreArgs = []string{"link", "set", "dev", intf, "master", bond}
	if res, err := cmd.Run(resPath, ipCmd, append(ipArgs, moreArgs...)...); err != nil {
		return res.Diagnostics(),
			fmt.Errorf("can't enslave member '%s' to bond '%s': %w", intf, bond, err)
	}

	moreArgs = []string{"link", "set", "dev", intf, "up"}
	if res, err := cmd.Run(resPath, ipCmd, append(ipArgs, moreArgs...)...); err != nil {
		return res.Diagnostics(),
			fmt.Errorf("can't bring enslaved member '%s' up: %w", intf, err)
	}

	return nil, nil
}

// releaseMember detaches intf from its bond. A member that has already been
// removed (e.g. externally) is treated as success.
func (r *LAG) releaseMember(resPath string, ipCmd string, ipArgs []string, intf string) (diag.Diagnostics, error) {
	moreArgs := []string{"link", "set", "dev", intf, "nomaster"}
	res, err := cmd.Run(resPath, ipCmd, append(ipArgs, moreArgs...)...)
	if err != nil {
		if errchecker.ContainsNone(err, intfNotFoundStrs) &&
			errchecker.DiagsNone(res.Diagnostics(), intfNotFoundStrs) {
			return res.Diagnostics(),
				fmt.Errorf("can't release member '%s' from bond: %w", intf, err)
		}
	}
	return nil, nil
}

// reconcileAddr brings the address configured on bond from old to new. There is
// no iproute2 "replace" verb, so a changed address is removed and re-added;
// without the explicit delete both addresses would linger.
func (r *LAG) reconcileAddr(resPath string, ipCmd string, ipArgs []string, bond string, old, new types.String) (diag.Diagnostics, error) {
	if !old.IsNull() && !old.IsUnknown() {
		moreArgs := []string{"addr", "del", old.ValueString(), "dev", bond}
		res, err := cmd.Run(resPath, ipCmd, append(ipArgs, moreArgs...)...)
		if err != nil {
			tolerable := append(append([]string{}, intfNotFoundStrs...), addrNotFoundStrs...)
			if errchecker.ContainsNone(err, tolerable) &&
				errchecker.DiagsNone(res.Diagnostics(), tolerable) {
				return res.Diagnostics(),
					fmt.Errorf("can't remove old address '%s' from bond '%s': %w", old.ValueString(), bond, err)
			}
		}
	}

	if !new.IsNull() && !new.IsUnknown() {
		addr := new.ValueString()
		if _, _, err := net.ParseCIDR(addr); err != nil {
			dgs := diag.Diagnostics{}
			dgs.AddError("Invalid address format",
				fmt.Sprintf("address must be in CIDR format: %s", err.Error()))
			return dgs, fmt.Errorf("invalid address '%s' for bond '%s': %w", addr, bond, err)
		}
		moreArgs := []string{"addr", "add", addr, "dev", bond}
		res, err := cmd.Run(resPath, ipCmd, append(ipArgs, moreArgs...)...)
		if err != nil {
			return res.Diagnostics(),
				fmt.Errorf("can't add address '%s' to bond '%s': %w", addr, bond, err)
		}
	}

	return nil, nil
}

func (r *LAG) readLAG(resPath string, ipCmd string, ipArgs []string, model *LAGModel) (diag.Diagnostics, error) {
	bond := model.Name.ValueString()

	// Check if the bond exists and get info.
	moreArgs := []string{"link", "show", bond}
	res, err := cmd.Run(resPath, ipCmd, append(ipArgs, moreArgs...)...)
	if err != nil {
		return res.Diagnostics(), fmt.Errorf("can't retrieve bond '%s' details: %w", bond, err)
	}

	// Parse output for MTU, state, and MAC address.
	lines := strings.Split(res.Stdout, "\n")
	// A real `ip link show` line always contains the flags section "<...>".
	// If it doesn't, the output is empty or unparseable: fail loudly with the
	// raw output instead of fabricating a "down" state.
	if len(lines) == 0 || !strings.Contains(lines[0], "<") {
		return res.Diagnostics(),
			fmt.Errorf("can't parse bond '%s' link details, unexpected output: %q", bond, res.Stdout)
	}
	// Parse first line: "5: bond0: <BROADCAST,MULTICAST,MASTER,UP,LOWER_UP> mtu 1500 ..."
	mtuRegex := regexp.MustCompile(`mtu (\d+)`)
	if matches := mtuRegex.FindStringSubmatch(lines[0]); len(matches) > 1 {
		x, err := strconv.ParseInt(matches[1], 10, 64)
		if err != nil {
			e := fmt.Errorf("invalid bond '%s' MTU value '%s': %w", bond, matches[1], err)
			d := diag.Diagnostics{}
			d.AddError("Can't find bond interface MTU value", e.Error())
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

	// Parse MAC address from second line: "    link/ether 00:11:22:33:44:55 brd ..."
	if len(lines) > 1 {
		macRegex := regexp.MustCompile(`link/ether\s+([0-9a-fA-F:]+)`)
		if matches := macRegex.FindStringSubmatch(lines[1]); len(matches) > 1 {
			model.MACAddress = types.StringValue(matches[1])
		}
	}

	// Get IP address(es) of the bond.
	moreArgs = []string{"addr", "show", bond}
	res, err = cmd.Run(resPath, ipCmd, append(ipArgs, moreArgs...)...)
	if err != nil {
		return res.Diagnostics(), fmt.Errorf("can't retrieve bond '%s' addresses: %w", bond, err)
	}
	// Look for IPv4 address in CIDR format: inet 192.168.1.1/24 brd ...
	addrRegex := regexp.MustCompile(`inet (\d+\.\d+\.\d+\.\d+/\d+)`)
	if matches := addrRegex.FindStringSubmatch(res.Stdout); len(matches) > 1 {
		if _, _, err := net.ParseCIDR(matches[1]); err == nil {
			model.IPv4Address = types.StringValue(matches[1])
		}
	}
	// Look for IPv6 address in CIDR format, skipping link-local (fe80::/10).
	addr6Regex := regexp.MustCompile(`inet6 ([0-9a-fA-F:]+/\d+)`)
	for _, match := range addr6Regex.FindAllStringSubmatch(res.Stdout, -1) {
		if len(match) > 1 {
			ip, _, err := net.ParseCIDR(match[1])
			if err != nil {
				continue
			}
			if !ip.IsLinkLocalUnicast() {
				model.IPv6Address = types.StringValue(match[1])
				break
			}
		}
	}

	// Reconcile the set of interfaces this resource explicitly enslaved.
	//
	// `ip link show master <bond>` lists EVERY member, but other resources may
	// attach members on their own (e.g. a zedamigo_tap with its own `master`).
	// Those foreign members must not land in `enslaved_interfaces`. We reconcile
	// only against the interfaces already recorded in the incoming model (prior
	// state on Read, the configured/plan set on Create/Update): keep the ones we
	// manage that are still members (so real drift is detected and self-heals on
	// the next in-place update), drop everything else.
	//
	// Unlike zedamigo_bridge we preserve the null-vs-empty distinction: only a
	// null/unknown incoming set becomes null. A known (even empty) set yields a
	// (possibly empty) set so that an explicit `enslaved_interfaces = []`
	// round-trips without a perpetual diff.
	if model.EnslavedInterfaces.IsNull() || model.EnslavedInterfaces.IsUnknown() {
		model.EnslavedInterfaces = types.SetNull(types.StringType)
	} else {
		moreArgs = []string{"link", "show", "master", bond}
		res, err = cmd.Run(resPath, ipCmd, append(ipArgs, moreArgs...)...)
		if err != nil {
			return res.Diagnostics(), fmt.Errorf("can't retrieve bond '%s' members: %w", bond, err)
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

		// Keep only managed interfaces that are still members of the bond.
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
		members, diags := types.SetValue(types.StringType, kept)
		if diags.HasError() {
			return diags, fmt.Errorf("can't build bond '%s' members set", bond)
		}
		model.EnslavedInterfaces = members
	}

	return nil, nil
}

// lagMembersSlice converts a member set to a slice, treating null/unknown as
// empty.
func lagMembersSlice(ctx context.Context, s types.Set) ([]string, diag.Diagnostics) {
	if s.IsNull() || s.IsUnknown() {
		return nil, nil
	}
	var out []string
	diags := s.ElementsAs(ctx, &out, false)
	return out, diags
}

// stringsDifference returns the elements of a that are not present in b.
func stringsDifference(a, b []string) []string {
	in := make(map[string]bool, len(b))
	for _, s := range b {
		in[s] = true
	}
	var out []string
	for _, s := range a {
		if !in[s] {
			out = append(out, s)
		}
	}
	return out
}
