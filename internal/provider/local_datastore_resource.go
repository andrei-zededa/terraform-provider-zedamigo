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
	State     types.String `tfsdk:"state"`
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
			"state": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Desired state of the HTTP server daemon",
				MarkdownDescription: undent.Md(`Desired state of the HTTP server daemon. Can be "running" or "stopped".
				Defaults to "running". The provider will automatically start or stop the daemon to match this state.`),
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

	// Set default state to "running" if not specified
	if data.State.IsNull() || data.State.ValueString() == "" {
		data.State = types.StringValue("running")
	}

	// Only start the daemon if state is "running"
	if data.State.ValueString() == "running" {
		if err := r.startLocalDatastore(d, &data); err != nil {
			resp.Diagnostics.AddError("LocalDatastore Resource Error",
				fmt.Sprintf("Failed to start HTTP server: %v", err))
			return
		}
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
	var plan LocalDatastoreModel
	var state LocalDatastoreModel

	// Read Terraform plan and current state
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d := r.getResourceDir(state.ID.ValueString())

	// Check if only the state field changed
	stateChanged := !plan.State.Equal(state.State)
	configChanged := !plan.Listen.Equal(state.Listen) ||
		!plan.StaticDir.Equal(state.StaticDir) ||
		!plan.BwLimit.Equal(state.BwLimit) ||
		!plan.Username.Equal(state.Username) ||
		!plan.Password.Equal(state.Password)

	// If configuration changed, we need to restart the daemon
	if configChanged {
		tflog.Info(ctx, "HTTP server configuration changed, restarting daemon...")

		// Stop the current daemon if running
		if err := r.stopLocalDatastore(d); err != nil {
			tflog.Warn(ctx, "Failed to stop HTTP server before restart", map[string]any{"error": err.Error()})
		}

		// Start with new configuration if desired state is running
		desiredState := plan.State.ValueString()
		if desiredState == "" {
			desiredState = "running"
		}

		if desiredState == "running" {
			if err := r.startLocalDatastore(d, &plan); err != nil {
				resp.Diagnostics.AddError("LocalDatastore Resource Update Error",
					fmt.Sprintf("Failed to restart HTTP server with new configuration: %v", err))
				return
			}
		}
		plan.State = types.StringValue(desiredState)
	} else if stateChanged {
		// Only state field changed
		desiredState := plan.State.ValueString()
		if desiredState == "" {
			desiredState = "running"
		}

		tflog.Info(ctx, "HTTP server state change requested", map[string]any{
			"from": state.State.ValueString(),
			"to":   desiredState,
		})

		if desiredState == "running" {
			if err := r.startLocalDatastore(d, &plan); err != nil {
				resp.Diagnostics.AddError("LocalDatastore Resource Update Error",
					fmt.Sprintf("Failed to start HTTP server: %v", err))
				return
			}
		} else if desiredState == "stopped" {
			if err := r.stopLocalDatastore(d); err != nil {
				resp.Diagnostics.AddError("LocalDatastore Resource Update Error",
					fmt.Sprintf("Failed to stop HTTP server: %v", err))
				return
			}
		}

		plan.State = types.StringValue(desiredState)
	}

	// Read back the current state to verify
	if diags, err := r.readLocalDatastore(d, &plan); err != nil {
		resp.Diagnostics.AddError("Failed to read LocalDatastore state after update", err.Error())
		resp.Diagnostics.Append(diags...)
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *LocalDatastore) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data LocalDatastoreModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d := r.getResourceDir(data.ID.ValueString())

	// Stop the HTTP server process if it's running.
	if err := r.stopLocalDatastore(d); err != nil {
		// Log as warning instead of error, since the daemon might already be stopped
		tflog.Warn(ctx, "Failed to stop HTTP server during delete", map[string]any{"error": err.Error()})
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

// startLocalDatastore starts the HTTP server daemon for the given resource
func (r *LocalDatastore) startLocalDatastore(d string, data *LocalDatastoreModel) error {
	srvCmd := os.Args[0]
	args := []string{"-pid-file", data.PIDFile.ValueString(), "-http-server"}

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

	if res, err := cmd.RunDetached(d, srvCmd, args...); err != nil {
		return fmt.Errorf("failed to start HTTP server: %w, diagnostics: %v", err, res.Diagnostics())
	}
	return nil
}

// stopLocalDatastore stops the HTTP server daemon for the given resource
func (r *LocalDatastore) stopLocalDatastore(d string) error {
	running, proc, err := readLocalDatastorePID(d)
	if err != nil {
		return fmt.Errorf("can't find HTTP server process: %w", err)
	}
	if !running {
		return nil // Already stopped
	}

	if err := proc.Kill(); err != nil {
		return fmt.Errorf("can't kill HTTP server process: %w", err)
	}
	return nil
}

func (r *LocalDatastore) readLocalDatastore(resPath string, model *LocalDatastoreModel) (diag.Diagnostics, error) {
	// Check if resource directory exists
	if _, err := os.Stat(resPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("resource directory does not exist")
	}

	// Determine desired state (default to "running" if not set)
	desiredState := "running"
	if !model.State.IsNull() && model.State.ValueString() != "" {
		desiredState = model.State.ValueString()
	}

	// Check if daemon is actually running
	running, _, _ := readLocalDatastorePID(resPath)
	actualState := "stopped"
	if running {
		actualState = "running"
	}

	// Self-healing: reconcile actual state with desired state
	if desiredState == "running" && actualState == "stopped" {
		// Daemon should be running but isn't - restart it
		tflog.Info(context.Background(), "HTTP server daemon is stopped but should be running, restarting...")
		if err := r.startLocalDatastore(resPath, model); err != nil {
			return nil, fmt.Errorf("failed to restart HTTP server: %w", err)
		}
		actualState = "running"
	} else if desiredState == "stopped" && actualState == "running" {
		// Daemon should be stopped but is running - stop it
		tflog.Info(context.Background(), "HTTP server daemon is running but should be stopped, stopping...")
		if err := r.stopLocalDatastore(resPath); err != nil {
			return nil, fmt.Errorf("failed to stop HTTP server: %w", err)
		}
		actualState = "stopped"
	}

	// Update state to match actual state
	model.State = types.StringValue(actualState)

	return nil, nil
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
