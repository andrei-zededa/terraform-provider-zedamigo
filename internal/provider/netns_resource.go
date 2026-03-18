// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/cmd"
	"github.com/davecgh/go-spew/spew"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

const (
	netnsDir = "netns"
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                = &NetNS{}
	_ resource.ResourceWithImportState = &NetNS{}
)

func NewNetNS() resource.Resource {
	return &NetNS{}
}

// NetNS defines the resource implementation.
type NetNS struct {
	providerConf *ZedAmigoProviderConfig
}

// NetNSModel describes the resource data model.
type NetNSModel struct {
	ID   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
}

func (r *NetNS) getResourceDir(id string) string {
	return filepath.Join(r.providerConf.LibPath, netnsDir, id)
}

func (r *NetNS) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_netns"
}

func (r *NetNS) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description:         "Network namespace",
		MarkdownDescription: "Create and manage a Linux network namespace using iproute2 commands.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				Description:         "Network namespace identifier",
				MarkdownDescription: "Network namespace identifier",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Name of the network namespace",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *NetNS) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
	tflog.Trace(ctx, "NetNS resource configure debugging", traceData)
}

func (r *NetNS) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data NetNSModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, err := newResourceID()
	if err != nil {
		resp.Diagnostics.AddError("NetNS Resource Error",
			fmt.Sprintf("Unable to generate a new resource ID: %s", err))
		return
	}
	data.ID = types.StringValue(id)

	d := r.getResourceDir(data.ID.ValueString())
	if err := os.MkdirAll(d, 0o700); err != nil {
		resp.Diagnostics.AddError("NetNS Resource Error",
			fmt.Sprintf("Unable to create resource specific directory: %s", err))
		return
	}
	if err := createTFBackPointer(d); err != nil {
		resp.Diagnostics.AddError("NetNS Resource Error",
			fmt.Sprintf("Unable to create resource specific file: %s", err))
		return
	}

	nsName := data.Name.ValueString()

	ipCmd := r.providerConf.IP
	ipArgs := []string{}
	if r.providerConf.UseSudo {
		ipCmd = r.providerConf.Sudo
		ipArgs = []string{"-n", r.providerConf.IP}
	}

	// Create the network namespace.
	moreArgs := []string{"netns", "add", nsName}
	res, err := cmd.Run(d, ipCmd, append(ipArgs, moreArgs...)...)
	if err != nil {
		resp.Diagnostics.AddError("NetNS Resource Error",
			"Unable to create a new network namespace.")
		resp.Diagnostics.Append(res.Diagnostics()...)
		return
	}

	tflog.Trace(ctx, "NetNS Resource created successfully")

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *NetNS) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data NetNSModel

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

	// List network namespaces and check if ours exists.
	moreArgs := []string{"netns", "list"}
	res, err := cmd.Run(d, ipCmd, append(ipArgs, moreArgs...)...)
	if err != nil {
		resp.Diagnostics.AddError("NetNS Resource Error",
			fmt.Sprintf("Unable to list network namespaces: %s", err))
		resp.Diagnostics.Append(res.Diagnostics()...)
		return
	}

	nsName := data.Name.ValueString()
	found := false
	for _, line := range strings.Split(res.Stdout, "\n") {
		// ip netns list output format: "name (id: N)" or just "name"
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == nsName {
			found = true
			break
		}
	}

	if !found {
		// Namespace was deleted outside Terraform: remove from state.
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *NetNS) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("NetNS Resource Update Error", "Update is not supported.")
}

func (r *NetNS) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data NetNSModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	nsName := data.Name.ValueString()
	d := r.getResourceDir(data.ID.ValueString())

	ipCmd := r.providerConf.IP
	ipArgs := []string{}
	if r.providerConf.UseSudo {
		ipCmd = r.providerConf.Sudo
		ipArgs = []string{"-n", r.providerConf.IP}
	}

	// Delete the network namespace.
	moreArgs := []string{"netns", "delete", nsName}
	res, err := cmd.Run(d, ipCmd, append(ipArgs, moreArgs...)...)
	if err != nil {
		errMsg := err.Error()
		// Idempotent: if the namespace doesn't exist, the delete is successful.
		if !strings.Contains(errMsg, "No such file") &&
			!strings.Contains(errMsg, "no such file") &&
			!strings.Contains(errMsg, "Cannot find") &&
			!strings.Contains(errMsg, "Invalid argument") {
			resp.Diagnostics.AddError("Failed to delete network namespace", err.Error())
			resp.Diagnostics.Append(res.Diagnostics()...)
			return
		}
	}

	if err := os.RemoveAll(d); err != nil {
		resp.Diagnostics.AddError("NetNS Resource Delete Error",
			fmt.Sprintf("Can't delete NetNS resource directory: %v", err))
		return
	}
}

func (r *NetNS) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
