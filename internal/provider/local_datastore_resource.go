// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/cmd"
	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/undent"
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
	"github.com/shirou/gopsutil/v4/process"
)

const (
	localDatastoresDir = "local_datastores"
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                = &LocalDatastore{}
	_ resource.ResourceWithImportState = &LocalDatastore{}
)

func NewLocalDatastore() resource.Resource {
	return &LocalDatastore{}
}

// LocalDatastore defines the resource implementation.
type LocalDatastore struct {
	providerConf *ZedAmigoProviderConfig
}

// LocalDatastoreModel describes the resource data model.
type LocalDatastoreModel struct {
	ID        types.String `tfsdk:"id"`
	Listen    types.String `tfsdk:"listen"`
	StaticDir types.String `tfsdk:"static_dir"`
	BwLimit   types.String `tfsdk:"bw_limit"`
	Username  types.String `tfsdk:"username"`
	Password  types.String `tfsdk:"password"`
	PIDFile   types.String `tfsdk:"pid_file"`
}

func (r *LocalDatastore) getResourceDir(id string) string {
	return filepath.Join(r.providerConf.LibPath, localDatastoresDir, id)
}

func (r *LocalDatastore) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_local_datastore"
}

func (r *LocalDatastore) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Local HTTP server for serving static files as a datastore",
		MarkdownDescription: undent.Md(`Create and manage a local HTTP server instance that serves static files.
		This can be used as a datastore for EVE-OS edge nodes or other purposes. The server supports
		bandwidth limiting and optional HTTP basic authentication.`),

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				Description:         "Local datastore resource identifier",
				MarkdownDescription: "Local datastore resource identifier.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"listen": schema.StringAttribute{
				Description: "Listen address (host:port)",
				MarkdownDescription: undent.Md(`Listen address in the format host:port.
				If not specified, defaults to :8080.`),
				Optional: true,
			},
			"static_dir": schema.StringAttribute{
				Description: "Directory to serve static files from",
				MarkdownDescription: undent.Md(`The directory path from which static files will be served.
				This is a required field.`),
				Required: true,
			},
			"bw_limit": schema.StringAttribute{
				Description: "Bandwidth limit (e.g., '2m', '2mb', '2M', '2MB', '2GB')",
				MarkdownDescription: undent.Md(`Bandwidth limit for the HTTP server. Can be specified with units
				like '2m', '2mb', '2M', '2MB', '2GB'. If not specified, defaults to '2GB'.`),
				Optional: true,
			},
			"username": schema.StringAttribute{
				Description: "Username for HTTP basic authentication (empty disables auth)",
				MarkdownDescription: undent.Md(`Username for HTTP basic authentication. If empty (default),
				authentication is disabled. If specified, password must also be provided.`),
				Optional:  true,
				Sensitive: true,
			},
			"password": schema.StringAttribute{
				Description: "Password for HTTP basic authentication",
				MarkdownDescription: undent.Md(`Password for HTTP basic authentication. Only used when
				username is also specified.`),
				Optional:  true,
				Sensitive: true,
			},
			"pid_file": schema.StringAttribute{
				Computed:    true,
				Description: "Process ID file",
			},
		},
	}
}

func (r *LocalDatastore) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
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
	tflog.Trace(ctx, "Local datastore resource configure debugging", traceData)
}

func (r *LocalDatastore) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data LocalDatastoreModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Validate that static_dir exists
	if _, err := os.Stat(data.StaticDir.ValueString()); err != nil {
		resp.Diagnostics.AddError("LocalDatastore Resource Error",
			fmt.Sprintf("Static directory does not exist: %s", err))
		return
	}

	u, err := uuid.NewV4()
	if err != nil {
		resp.Diagnostics.AddError("LocalDatastore Resource Error",
			fmt.Sprintf("Unable to generate a new UUID: %s", err))
		return
	}
	data.ID = types.StringValue(u.String())

	d := r.getResourceDir(data.ID.ValueString())
	if err := os.MkdirAll(d, 0o700); err != nil {
		resp.Diagnostics.AddError("LocalDatastore Resource Error",
			fmt.Sprintf("Unable to create resource specific directory: %s", err))
		return
	}
	if err := createTFBackPointer(d); err != nil {
		resp.Diagnostics.AddError("LocalDatastore Resource Error",
			fmt.Sprintf("Unable to create resource specific file: %s", err))
		return
	}

	pidFile := filepath.Join(d, "pid")
	data.PIDFile = types.StringValue(pidFile)

	// Build command arguments
	srvCmd := os.Args[0]
	args := []string{"-pid-file", pidFile, "-http-server"}

	// Add optional arguments
	if !data.Listen.IsNull() && data.Listen.ValueString() != "" {
		args = append(args, "-hs.listen", data.Listen.ValueString())
	}

	args = append(args, "-hs.static-dir", data.StaticDir.ValueString())

	if !data.BwLimit.IsNull() && data.BwLimit.ValueString() != "" {
		args = append(args, "-hs.bw-limit", data.BwLimit.ValueString())
	}

	if !data.Username.IsNull() && data.Username.ValueString() != "" {
		args = append(args, "-hs.username", data.Username.ValueString())
	}

	if !data.Password.IsNull() && data.Password.ValueString() != "" {
		args = append(args, "-hs.password", data.Password.ValueString())
	}

	// Run the HTTP server in detached mode
	if res, err := cmd.RunDetached(d, srvCmd, args...); err != nil {
		resp.Diagnostics.AddError("LocalDatastore Resource Error",
			"Failed to run HTTP server")
		resp.Diagnostics.Append(res.Diagnostics()...)
		return
	}

	// Read the LocalDatastore current state.
	if diags, err := r.readLocalDatastore(d, &data); err != nil {
		resp.Diagnostics.AddError("Failed to read LocalDatastore state", err.Error())
		resp.Diagnostics.Append(diags...)
		return
	}

	tflog.Trace(ctx, "LocalDatastore Resource created successfully")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *LocalDatastore) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data LocalDatastoreModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d := r.getResourceDir(data.ID.ValueString())

	// Read the LocalDatastore current state.
	if diags, err := r.readLocalDatastore(d, &data); err != nil {
		if os.IsNotExist(err) {
			resp.State.RemoveResource(ctx)
			return
		}

		resp.Diagnostics.AddError("Failed to read LocalDatastore state", err.Error())
		resp.Diagnostics.Append(diags...)
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *LocalDatastore) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data LocalDatastoreModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.AddError("LocalDatastore Resource Update Error", "Update is not supported.")
}

func (r *LocalDatastore) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data LocalDatastoreModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d := r.getResourceDir(data.ID.ValueString())

	// Kill the HTTP server process if it's running.
	running, proc, err := readLocalDatastorePID(d)
	if err != nil {
		resp.Diagnostics.AddError("LocalDatastore Resource Delete Error",
			fmt.Sprintf("Can't find details of HTTP server process: %v", err))
		return
	}
	if running {
		if err := proc.Kill(); err != nil {
			resp.Diagnostics.AddError("LocalDatastore Resource Delete Error",
				fmt.Sprintf("Can't kill HTTP server process: %v", err))
			return
		}
	}

	if err := os.RemoveAll(d); err != nil {
		resp.Diagnostics.AddError("LocalDatastore Resource Delete Error",
			fmt.Sprintf("Can't delete LocalDatastore resource directory: %v", err))
		return
	}
}

func (r *LocalDatastore) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *LocalDatastore) readLocalDatastore(resPath string, model *LocalDatastoreModel) (diag.Diagnostics, error) {
	// Verify the process is still running
	_, _, err := readLocalDatastorePID(resPath)
	return nil, err
}

func readLocalDatastorePID(path string) (bool, *process.Process, error) {
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
