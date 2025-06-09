// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
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
	diskImagesDir = "disk_images"
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                = &DiskImage{}
	_ resource.ResourceWithImportState = &DiskImage{}
)

func NewDiskImage() resource.Resource {
	return &DiskImage{}
}

// DiskImage defines the resource implementation.
type DiskImage struct {
	providerConf *ZedAmigoProviderConfig
}

// DiskImageModel describes the resource data model.
type DiskImageModel struct {
	ID        types.String `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	SizeMB    types.Int64  `tfsdk:"size_mb"`
	Filename  types.String `tfsdk:"filename"`
	Usedbytes types.Int64  `tfsdk:"used_bytes"`
}

func (r *DiskImage) getResourceDir(id string) string {
	return filepath.Join(r.providerConf.LibPath, diskImagesDir, id)
}

func (r *DiskImage) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_disk_image"
}

func (r *DiskImage) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Disk image",
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Disk image",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				Description:         "Disk image identifier",
				MarkdownDescription: "Disk image identifier",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description:         "Disk image name (also the file-name)",
				MarkdownDescription: "Disk image name (also the file-name)",
				Optional:            false,
				Required:            true,
			},
			"size_mb": schema.Int64Attribute{
				Description:         "Disk image size in MB (megabytes, old-style power of 2)",
				MarkdownDescription: "Disk image size in MB (megabytes, old-style power of 2)",
				Optional:            false,
				Required:            true,
			},
			"filename": schema.StringAttribute{
				Description:         "Full path/filename of the disk image",
				MarkdownDescription: "Full path/filename of the disk image",
				Computed:            true,
			},
			"used_bytes": schema.Int64Attribute{
				Description:         "Current size of the disk image/how many bytes have been written out of the total `size_mb` capacity",
				MarkdownDescription: "Current size of the disk image/how many bytes have been written out of the total `size_mb` capacity",
				Computed:            true,
			},
		},
	}
}

func (r *DiskImage) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
}

func (r *DiskImage) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data DiskImageModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	u, err := uuid.NewV4()
	if err != nil {
		resp.Diagnostics.AddError("Disk Image Resource Error",
			fmt.Sprintf("Unable to generate a new UUID: %s", err))
		return
	}
	data.ID = types.StringValue(u.String())
	if resp.Diagnostics.HasError() {
		return
	}

	d := r.getResourceDir(data.ID.ValueString())
	if err := os.MkdirAll(d, 0o700); err != nil {
		resp.Diagnostics.AddError("Disk Image Resource Error",
			fmt.Sprintf("Unable to create resource specific directory: %s", err))
		return
	}
	if err := createTFBackPointer(d); err != nil {
		resp.Diagnostics.AddError("Disk Image Resource Error",
			fmt.Sprintf("Unable to create resource specific file: %s", err))
		return
	}
	i := fmt.Sprintf("%s.disk_img.qcow2", filepath.Join(d, data.Name.ValueString()))
	res, err := cmd.Run(d, r.providerConf.QemuImg, "create", "-f", "qcow2", i,
		fmt.Sprintf("%sM", data.SizeMB.String()))
	if err != nil {
		resp.Diagnostics.AddError("Disk Image Resource Error",
			"Unable to create a new disk image.")
		resp.Diagnostics.Append(res.Diagnostics()...)
		return
	}

	tflog.Trace(ctx, "Disk Image Resource created succesfully")

	qi, err := readDiskImage(r.providerConf, r.getResourceDir(data.ID.ValueString()),
		data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Disk Image Resource Error",
			fmt.Sprintf("Can't read back disk image resource: %v", err))
		return
	}
	data.Filename = types.StringValue(qi.Filename)
	data.Usedbytes = types.Int64Value(qi.ActualSize)

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// qcowInfo represents the relevant information returned by qemu-img info.
type qcowInfo struct {
	VirtualSize int64  `json:"virtual-size"`
	Filename    string `json:"filename"`
	Format      string `json:"format"`
	ActualSize  int64  `json:"actual-size"`
}

func readDiskImage(providerConf *ZedAmigoProviderConfig, path, name string) (qcowInfo, error) {
	qi := qcowInfo{}

	i := fmt.Sprintf("%s.disk_img.qcow2", filepath.Join(path, name))

	res, err := cmd.Run(path, providerConf.QemuImg, "info", "--output=json", i)
	if err != nil {
		return qi, fmt.Errorf("qemu-img command failed: %w", err)
	}

	if err := json.Unmarshal([]byte(res.Stdout), &qi); err != nil {
		return qi, fmt.Errorf("failed to parse qemu-img JSON output: %w", err)
	}

	if qi.Format != "qcow2" {
		return qi, fmt.Errorf("image is not in qcow2 format, got: %s", qi.Format)
	}

	return qi, nil
}

func (r *DiskImage) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data DiskImageModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	qi, err := readDiskImage(r.providerConf, r.getResourceDir(data.ID.ValueString()),
		data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Disk Image Resource Read Error",
			fmt.Sprintf("Can't read back disk image resource: %v", err))
		return
	}
	data.Filename = types.StringValue(qi.Filename)
	data.Usedbytes = types.Int64Value(qi.ActualSize)

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DiskImage) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data DiskImageModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.AddError("Disk Image Resource Update Error", "Update is not supported.")
}

func (r *DiskImage) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data DiskImageModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d := r.getResourceDir(data.ID.ValueString())
	if err := os.RemoveAll(d); err != nil {
		resp.Diagnostics.AddError("Disk Image Resource Delete Error",
			fmt.Sprintf("Can't delete disk image directory: %v", err))
		return
	}
}

func (r *DiskImage) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// createTFBackPointer will create a file that points to the source directory
// of the TF configuration that created a specific resource. This is useful for
// finding orphaned resources created by old/no-longer-existing configs.
func createTFBackPointer(path string) error {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("createTFBackPointer: %w", err)
	}

	f := filepath.Join(path, "config_source_dir.tf")
	if err := os.WriteFile(f, []byte("# "+wd), 0o640); err != nil {
		return fmt.Errorf("createTFBackPointer: %w", err)
	}

	return nil
}
