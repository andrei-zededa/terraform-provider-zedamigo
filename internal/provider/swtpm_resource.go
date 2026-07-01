// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	_ "embed"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	tpmc "github.com/google/go-tpm-tools/client"
)

const (
	swtpmsDir = "swtmps"
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                = &SwTPM{}
	_ resource.ResourceWithImportState = &SwTPM{}
)

//go:embed process_monitor.bash
var processMonitor []byte

func NewSwTPM() resource.Resource {
	return &SwTPM{}
}

// SwTPM defines the resource implementation.
type SwTPM struct {
	providerConf *ZedAmigoProviderConfig
}

// SwTPMModel describes the resource data model.
type SwTPMModel struct {
	ID      types.String `tfsdk:"id"`
	Name    types.String `tfsdk:"name"`
	EK      types.String `tfsdk:"endorsment_key"`
	Socket  types.String `tfsdk:"socket"`
	Running types.Bool   `tfsdk:"running"`
}

func (r *SwTPM) getResourceDir(id string) string {
	return filepath.Join(r.providerConf.LibPath, swtpmsDir, id)
}

func (r *SwTPM) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_swtpm"
}

func (r *SwTPM) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "SwTPM",
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "SwTPM",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:         "SwTPM identifier",
				MarkdownDescription: "SwTPM identifier",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
				Computed: true,
			},
			"name": schema.StringAttribute{
				Description:         "SwTPM name",
				MarkdownDescription: "SwTPM name",
				Optional:            false,
				Required:            true,
			},
			"endorsment_key": schema.StringAttribute{
				Description:         "TPM endorsment key",
				MarkdownDescription: "TPM endorsment key",
				Computed:            true,
			},
			"socket": schema.StringAttribute{
				Description:         "UNIX socket for this SwTPM process",
				MarkdownDescription: "UNIX socket for this SwTPM process",
				Computed:            true,
			},
			"running": schema.BoolAttribute{
				Description:         "Running state of the SwTPM process",
				MarkdownDescription: "Running state of the SwTPM process",
				Computed:            true,
			},
		},
	}
}

func (r *SwTPM) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *SwTPM) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data SwTPMModel

	// Read Terraform plan data into the model/
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	data.ID = types.StringValue(sum2ID(data.Name.ValueString()))

	d := r.getResourceDir(data.ID.ValueString())
	if err := r.providerConf.Exec.MkdirAll(ctx, filepath.Join(d, "state"), 0o700); err != nil {
		resp.Diagnostics.AddError("SwTPM Resource Error",
			fmt.Sprintf("Unable to create resource specific directory: %s", err))
		return
	}
	if err := createTFBackPointer(ctx, r.providerConf.Exec, d); err != nil {
		resp.Diagnostics.AddError("Disk Image Resource Error",
			fmt.Sprintf("Unable to create resource specific file: %s", err))
		return
	}

	if running, _, _ := readMonitorPID(ctx, r.providerConf, d); running {
		resp.Diagnostics.AddError("SwTPM Resource Error",
			fmt.Sprintf("Process monitor is already running"))
		return
	}

	if err := r.providerConf.Exec.WriteFile(ctx, filepath.Join(d, "process_monitor.bash"), processMonitor, 0o755); err != nil {
		resp.Diagnostics.AddError("SwTPM Resource Error",
			fmt.Sprintf("Failed to write file: %s", err))
		return
	}

	socketPath := filepath.Join(d, "swtpm.socket")
	cmdArgs := []string{
		filepath.Join(d, "process_monitor.bash"),
		"-p", filepath.Join(d, "process_monitor.pid"),
		r.providerConf.Swtpm,
		"socket",
		// "--daemon", // NOTE: Might interact badly with the process monitor script.
		"--pid", fmt.Sprintf("file=%s", filepath.Join(d, "swtpm.pid")),
		"--log", fmt.Sprintf("level=20,file=%s", filepath.Join(d, "swtpm.log")),
		"--ctrl", fmt.Sprintf("type=unixio,path=%s", socketPath),
		"--tpmstate", fmt.Sprintf("dir=%s", filepath.Join(d, "state")),
		"--tpm2",
		"--flags", "not-need-init",
	}

	res, err := r.providerConf.Exec.RunDetached(ctx, d, r.providerConf.Bash, cmdArgs...)
	if err != nil {
		resp.Diagnostics.AddError("SwTPM Resource Error",
			"Failed to run a swtpm process")
		resp.Diagnostics.Append(res.Diagnostics()...)
		return
	}

	data.Socket = types.StringValue(socketPath)

	tflog.Trace(ctx, "SwTPM Resource created succesfully")

	time.Sleep(2 * time.Second)

	x, ek, err := readSwTPM(ctx, r.providerConf, r.getResourceDir(data.ID.ValueString()))
	if err != nil {
		resp.Diagnostics.AddWarning("SwTPM Resource Read Error",
			fmt.Sprintf("Can't read swtpm process details: %v", err))

		return
	}

	data.EK = types.StringValue(ek)
	data.Running = types.BoolValue(x)

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func readMonitorPID(ctx context.Context, pConf *ZedAmigoProviderConfig, path string) (bool, int, error) {
	pidPath := filepath.Join(path, "process_monitor.pid")
	x, err := pConf.Exec.ReadFile(ctx, pidPath)
	if err != nil {
		return false, 0, fmt.Errorf("%w", err)
	}

	pid, err := strconv.ParseInt(string(bytes.TrimSpace(x)), 10, 32)
	if err != nil {
		return false, 0, fmt.Errorf("%w", err)
	}

	// The PID must be alive AND be our process monitor script.
	monScript := filepath.Join(path, "process_monitor.bash")
	running, err := pConf.Exec.IsRunning(ctx, int(pid), monScript)
	if err != nil {
		return false, int(pid), fmt.Errorf("%w", err)
	}

	return running, int(pid), nil
}

func readSwTPMPID(ctx context.Context, pConf *ZedAmigoProviderConfig, path string) (bool, int, error) {
	pidPath := filepath.Join(path, "swtpm.pid")
	x, err := pConf.Exec.ReadFile(ctx, pidPath)
	if err != nil {
		return false, 0, fmt.Errorf("%w", err)
	}

	pid, err := strconv.ParseInt(string(bytes.TrimSpace(x)), 10, 32)
	if err != nil {
		return false, 0, fmt.Errorf("%w", err)
	}

	// The PID must be alive AND be the swtpm binary.
	running, err := pConf.Exec.IsRunning(ctx, int(pid), pConf.Swtpm)
	if err != nil {
		return false, int(pid), fmt.Errorf("%w", err)
	}

	return running, int(pid), nil
}

func readSwTPM(ctx context.Context, pConf *ZedAmigoProviderConfig, path string) (bool, string, error) {
	return true, "", nil

	// TODO: Unreachable code, see above.
	socketPath := filepath.Join(path, "swtpm.socket")
	conn, err := pConf.Exec.Dial(ctx, "unix", socketPath, 5*time.Second)
	if err != nil {
		return false, "", fmt.Errorf("failed to dial swtpm UNIX socket: %v", err)
	}
	defer conn.Close()

	// Get the Endorsement Key (ECC version).
	ek, err := tpmc.EndorsementKeyECC(conn)
	if err != nil {
		return false, "", fmt.Errorf("failed to get endorsement key: %v", err)
	}
	defer ek.Close()

	// You can access the public key.
	pubKey := ek.PublicKey()
	s := fmt.Sprintf("EK public key type: %T and value: %s", pubKey, pubKey)

	return true, s, nil
}

func (r *SwTPM) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data SwTPMModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	x, ek, err := readSwTPM(ctx, r.providerConf, r.getResourceDir(data.ID.ValueString()))
	if err != nil {
		resp.Diagnostics.AddWarning("SwTPM Resource Read Warning",
			fmt.Sprintf("Treating this as a warning since most likely"+
				" the corresponding SwTPM process is not running anymore:"+
				" %v", err))
		data.Running = types.BoolValue(false)
	}

	data.EK = types.StringValue(ek)
	data.Running = types.BoolValue(x)

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SwTPM) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data SwTPMModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.AddError("SwTPM Resource Update Error", "Update is not supported.")
}

func (r *SwTPM) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data SwTPMModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d := r.getResourceDir(data.ID.ValueString())

	running, swtpmPID, err := readSwTPMPID(ctx, r.providerConf, d)
	if err != nil {
		resp.Diagnostics.AddWarning("Treating SwTPM process related error as a warning", fmt.Sprintf("%v", err))
	}

	if running {
		pmrun, pmPID, err := readMonitorPID(ctx, r.providerConf, d)
		if err != nil {
			resp.Diagnostics.AddWarning("Treating SwTPM process monitor related error as a warning", fmt.Sprintf("%v", err))
		}
		// TODO: Seems like the process monitor script doesn't exit as expected after forwarding
		// the signal to the child.
		if pmrun {
			if err := r.providerConf.Exec.Kill(ctx, pmPID, syscall.SIGKILL); err != nil {
				resp.Diagnostics.AddError("Can't kill SwTPM process monitor", fmt.Sprintf("%v", err))
				return
			}
		} else {
			if err := r.providerConf.Exec.Kill(ctx, swtpmPID, syscall.SIGKILL); err != nil {
				resp.Diagnostics.AddError("Can't kill SwTPM process", fmt.Sprintf("%v", err))
				return
			}
		}
	}

	if err := r.providerConf.Exec.Remove(ctx, d); err != nil {
		resp.Diagnostics.AddError("SwTPM Resource Delete Error",
			fmt.Sprintf("Can't delete resource directory: %v", err))
		return
	}
}

func (r *SwTPM) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
