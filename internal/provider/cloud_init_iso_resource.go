// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/cmd"
	"github.com/gofrs/uuid/v5"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

const (
	cloudInitIsosDir = "cloud_init_isos"
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                = &CloudInitISO{}
	_ resource.ResourceWithImportState = &CloudInitISO{}
)

func NewCloudInitISO() resource.Resource {
	return &CloudInitISO{}
}

// CloudInitISO defines the resource implementation.
type CloudInitISO struct {
	providerConf *ZedAmigoProviderConfig
}

// CloudInitISOModel describes the resource data model.
type CloudInitISOModel struct {
	ID            types.String `tfsdk:"id"`
	Name          types.String `tfsdk:"name"`
	Filename      types.String `tfsdk:"filename"`
	UserData      types.String `tfsdk:"user_data"`
	MetaData      types.String `tfsdk:"meta_data"`
	NetworkConfig types.String `tfsdk:"network_config"`
}

func (r *CloudInitISO) getResourceDir(id string) string {
	return filepath.Join(r.providerConf.LibPath, cloudInitIsosDir, id)
}

func (r *CloudInitISO) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cloud_init_iso"
}

func (r *CloudInitISO) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Cloud Init ISO",
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Create a Cloud Init ISO (with user-data, meta-data and network-config)" +
			" that can be attached to a VM. Requires `genisoimage` (part of the cdrkit package)" +
			" to be installed.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				Description:         "Cloud Init ISO identifier",
				MarkdownDescription: "Cloud Init ISO identifier",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description:         "Cloud Init ISO name (also the file-name)",
				MarkdownDescription: "Cloud Init ISO name (also the file-name)",
				Optional:            false,
				Required:            true,
			},
			"filename": schema.StringAttribute{
				Description:         "Full path/filename of the ISO image",
				MarkdownDescription: "Full path/filename of the ISO image",
				Computed:            true,
			},
			"user_data": schema.StringAttribute{
				Description:         "The contents of the `user-data` file: https://cloudinit.readthedocs.io/en/latest/explanation/format.html",
				MarkdownDescription: "The contents of the `user-data` file: https://cloudinit.readthedocs.io/en/latest/explanation/format.html",
				Optional:            false,
				Required:            true,
			},
			"meta_data": schema.StringAttribute{
				Description:         "The contents of the `meta-data` file.",
				MarkdownDescription: "The contents of the `meta-data` file.",
				Optional:            false,
				Required:            true,
			},
			"network_config": schema.StringAttribute{
				Description:         "The contents of the `network-config` file.",
				MarkdownDescription: "The contents of the `network-config` file.",
				Optional:            false,
				Required:            true,
			},
		},
	}
}

func (r *CloudInitISO) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
		if resp.Diagnostics.HasError() {
			return
		}
	}

	if len(conf.GenISOImage) == 0 {
		resp.Diagnostics.AddError(
			"Missing `genisoimage` command (required for the Cloud Init ISO resource).",
			fmt.Sprintf("The `genisoimage` command is required for the Cloud Init ISO resource. It is part of the `cdrkit` package, please install it."),
		)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	r.providerConf = conf
}

func (r *CloudInitISO) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data CloudInitISOModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	u, err := uuid.NewV4()
	if err != nil {
		resp.Diagnostics.AddError("Cloud Init ISO Resource Error",
			fmt.Sprintf("Unable to generate a new UUID: %s", err))
		return
	}
	data.ID = types.StringValue(u.String())
	if resp.Diagnostics.HasError() {
		return
	}

	d := r.getResourceDir(data.ID.ValueString())
	if err := os.MkdirAll(filepath.Join(d, "cloud-init"), 0o700); err != nil {
		resp.Diagnostics.AddError("Cloud Init ISO Resource Error",
			fmt.Sprintf("Unable to create resource specific directory: %s", err))
		return
	}
	if err := createTFBackPointer(d); err != nil {
		resp.Diagnostics.AddError("Cloud Init ISO Resource Error",
			fmt.Sprintf("Unable to create resource specific file: %s", err))
		return
	}
	if err := os.WriteFile(filepath.Join(d, "cloud-init", "user-data"), []byte(data.UserData.ValueString()), 0o600); err != nil {
		resp.Diagnostics.AddError("Cloud Init ISO Resource Error",
			fmt.Sprintf("Unable to create resource specific file `user-data`: %s", err))
		return
	}
	if err := os.WriteFile(filepath.Join(d, "cloud-init", "meta-data"), []byte(data.MetaData.ValueString()), 0o600); err != nil {
		resp.Diagnostics.AddError("Cloud Init ISO Resource Error",
			fmt.Sprintf("Unable to create resource specific file `meta-data`: %s", err))
		return
	}
	if err := os.WriteFile(filepath.Join(d, "cloud-init", "network-config"), []byte(data.NetworkConfig.ValueString()), 0o600); err != nil {
		resp.Diagnostics.AddError("Cloud Init ISO Resource Error",
			fmt.Sprintf("Unable to create resource specific file `network-config`: %s", err))
		return
	}

	i := fmt.Sprintf("%s.iso", filepath.Join(d, data.Name.ValueString()))
	res, err := cmd.Run(d, r.providerConf.GenISOImage, "-output", i,
		"-volid", "cidata", "-joliet", "-rock",
		filepath.Join(d, "cloud-init", "user-data"),
		filepath.Join(d, "cloud-init", "meta-data"),
		filepath.Join(d, "cloud-init", "network-config"))
	if err != nil {
		resp.Diagnostics.AddError("Cloud init ISO Resource Error",
			"Unable to create a new ISO image.")
		resp.Diagnostics.Append(res.Diagnostics()...)
		return
	}
	data.Filename = types.StringValue(i)

	tflog.Trace(ctx, "Cloud Init ISO Resource created succesfully")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *CloudInitISO) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data CloudInitISOModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d := r.getResourceDir(data.ID.ValueString())
	i := fmt.Sprintf("%s.iso", filepath.Join(d, data.Name.ValueString()))

	data.Filename = types.StringValue(i)

	// TODO: We need to at least check that the ISO file actually exists,
	// additionally we might verify with `isoinfo` what it actually contains.

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *CloudInitISO) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data CloudInitISOModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// TODO: What to do here ?

	resp.Diagnostics.AddError("Cloud init ISO Resource Update Error", "Update is not supported.")
}

func (r *CloudInitISO) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data CloudInitISOModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d := r.getResourceDir(data.ID.ValueString())
	if err := os.RemoveAll(d); err != nil {
		resp.Diagnostics.AddError("Cloud init ISO Resource Delete Error",
			fmt.Sprintf("Can't delete disk image directory: %v", err))
		return
	}
}

func (r *CloudInitISO) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
