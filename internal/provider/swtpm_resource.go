// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	_ "embed"

	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/exec"
	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/undent"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

const (
	// swtpmsDir keeps its historical typo on purpose: renaming it would
	// orphan the swtpm processes and TPM state of resources created by
	// older builds of the provider.
	swtpmsDir = "swtmps"

	swtpmMonitorScript  = "process_monitor.bash"
	swtpmMonitorPIDFile = "process_monitor.pid"
	swtpmPIDFile        = "swtpm.pid"
	swtpmSocketFile     = "swtpm.socket"
	swtpmLogFile        = "swtpm.log"
	swtpmStateDir       = "state"

	// swtpmStartTimeout caps how long Create (and the self-healing Read)
	// waits for the swtpm process and its control socket to come up.
	swtpmStartTimeout = 10 * time.Second
	// swtpmStopTimeout caps how long the graceful stop path (SIGTERM to the
	// process monitor) is given before escalating to SIGKILL.
	swtpmStopTimeout = 5 * time.Second
	// swtpmLogTailBytes caps the swtpm log excerpt included in diagnostics.
	swtpmLogTailBytes = 1024

	swtpmPollInterval = 200 * time.Millisecond
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
	ID     types.String `tfsdk:"id"`
	Name   types.String `tfsdk:"name"`
	Socket types.String `tfsdk:"socket"`
	State  types.String `tfsdk:"state"`
}

func (r *SwTPM) getResourceDir(id string) string {
	return filepath.Join(r.providerConf.LibPath, swtpmsDir, id)
}

func (r *SwTPM) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_swtpm"
}

func (r *SwTPM) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Software TPM (swtpm) instance providing a virtual TPM 2.0 device for edge node VMs",
		MarkdownDescription: undent.Md(`
		Create and manage a software TPM instance: an [swtpm](https://github.com/stefanberger/swtpm)
		process emulating a TPM 2.0 device. The |socket| attribute is the QEMU control
		channel UNIX socket, meant to be used as the |swtpm_socket| attribute of a
		|zedamigo_edge_node| or |zedamigo_installed_edge_node| resource.

		The TPM state is stored in a directory under the provider |lib_path|, so the
		emulated TPM keeps its identity (endorsement keys, sealed data, ...) for the
		whole lifetime of the resource, across process restarts.

		QEMU terminates the attached swtpm process every time the VM shuts down (on a
		graceful exit it sends a shutdown command over the control channel, which swtpm
		always honors). Because of that the swtpm process runs under a small process
		monitor which restarts it automatically, so the same TPM (with the same state)
		can serve, for example, first the VM that runs the EVE-OS installer and then
		the VM that boots the installed system.`),

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				Description:         "SwTPM resource identifier",
				MarkdownDescription: "SwTPM resource identifier.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "SwTPM instance name",
				MarkdownDescription: undent.Md(`
				Name of the SwTPM instance. Changing it forces the creation of a new
				resource, which means a new TPM with a new identity and empty state.`),
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"socket": schema.StringAttribute{
				Computed:    true,
				Description: "UNIX socket (QEMU control channel) of this swtpm process",
				MarkdownDescription: undent.Md(`
				UNIX socket (QEMU control channel) of this swtpm process. Use it as the
				|swtpm_socket| attribute of an edge node resource.`),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"state": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Desired state of the swtpm process",
				MarkdownDescription: undent.Md(`
				Desired state of the swtpm process. Can be |running| or |stopped|.
				Defaults to |running|. The provider will automatically start or stop the
				process to match this state.`),
				Validators: []validator.String{
					stringvalidator.OneOf("running", "stopped"),
				},
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

	// Read Terraform plan data into the model.
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if r.providerConf.Swtpm == "" {
		resp.Diagnostics.AddError("SwTPM Resource Error",
			"The `swtpm` executable was not found on the target host when the provider was configured.")
		return
	}

	id, err := newResourceID()
	if err != nil {
		resp.Diagnostics.AddError("SwTPM Resource Error",
			fmt.Sprintf("Unable to generate a new resource ID: %s", err))
		return
	}
	data.ID = types.StringValue(id)

	d := r.getResourceDir(id)
	if err := r.setupResourceDir(ctx, d); err != nil {
		resp.Diagnostics.AddError("SwTPM Resource Error", err.Error())
		return
	}
	data.Socket = types.StringValue(filepath.Join(d, swtpmSocketFile))

	// Set default state to "running" if not specified.
	if data.State.IsNull() || data.State.ValueString() == "" {
		data.State = types.StringValue("running")
	}

	// Reconcile the (fresh, stopped) resource with the desired state; this
	// starts the swtpm process stack and waits for it to become ready.
	if err := r.readSwTPM(ctx, d, &data); err != nil {
		resp.Diagnostics.AddError("SwTPM Resource Error", err.Error())
		return
	}

	tflog.Trace(ctx, "SwTPM Resource created successfully")

	// Save data into Terraform state.
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SwTPM) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data SwTPMModel

	// Read Terraform prior state data into the model.
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d := r.getResourceDir(data.ID.ValueString())

	if err := r.readSwTPM(ctx, d, &data); err != nil {
		if strings.Contains(err.Error(), "does not exist") {
			resp.State.RemoveResource(ctx)
			return
		}

		resp.Diagnostics.AddError("Failed to read SwTPM state", err.Error())
		return
	}

	// Save updated data into Terraform state.
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SwTPM) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan SwTPMModel
	var state SwTPMModel

	// Read Terraform plan and current state.
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d := r.getResourceDir(state.ID.ValueString())

	// `name` requires replacement, so only `state` can change here.
	if plan.State.IsNull() || plan.State.ValueString() == "" {
		plan.State = types.StringValue("running")
	}
	if !plan.State.Equal(state.State) {
		tflog.Info(ctx, "swtpm state change requested", map[string]any{
			"from": state.State.ValueString(),
			"to":   plan.State.ValueString(),
		})
	}

	// Reconcile the actual processes with the desired state.
	if err := r.readSwTPM(ctx, d, &plan); err != nil {
		resp.Diagnostics.AddError("SwTPM Resource Update Error", err.Error())
		return
	}

	// Save updated data into Terraform state.
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *SwTPM) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data SwTPMModel

	// Read Terraform prior state data into the model.
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d := r.getResourceDir(data.ID.ValueString())

	// Deleting the resource directory while the processes are still alive
	// would leave the monitor respawning swtpm against a missing state dir
	// forever, so a stop failure aborts the delete.
	if err := r.stopSwTPM(ctx, d); err != nil {
		resp.Diagnostics.AddError("SwTPM Resource Delete Error",
			fmt.Sprintf("Can't stop the swtpm process(es), not deleting the resource directory, retry the destroy: %v", err))
		return
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

// setupResourceDir creates the per-resource directory layout: the TPM state
// sub-directory, the TF config back-pointer and the process monitor script.
func (r *SwTPM) setupResourceDir(ctx context.Context, d string) error {
	if err := r.providerConf.Exec.MkdirAll(ctx, filepath.Join(d, swtpmStateDir), 0o700); err != nil {
		return fmt.Errorf("unable to create resource specific directory: %w", err)
	}
	if err := createTFBackPointer(ctx, r.providerConf.Exec, d); err != nil {
		return fmt.Errorf("unable to create resource specific file: %w", err)
	}
	if err := r.providerConf.Exec.WriteFile(ctx, filepath.Join(d, swtpmMonitorScript), processMonitor, 0o755); err != nil {
		return fmt.Errorf("unable to write the process monitor script: %w", err)
	}

	return nil
}

// startSwTPM starts the process monitor which in turn starts (and restarts,
// see the resource description) the swtpm process.
func (r *SwTPM) startSwTPM(ctx context.Context, d string) error {
	cmdArgs := []string{
		filepath.Join(d, swtpmMonitorScript),
		"-p", filepath.Join(d, swtpmMonitorPIDFile),
		r.providerConf.Swtpm,
		"socket",
		"--pid", fmt.Sprintf("file=%s", filepath.Join(d, swtpmPIDFile)),
		"--log", fmt.Sprintf("level=20,file=%s", filepath.Join(d, swtpmLogFile)),
		"--ctrl", fmt.Sprintf("type=unixio,path=%s", filepath.Join(d, swtpmSocketFile)),
		"--tpmstate", fmt.Sprintf("dir=%s", filepath.Join(d, swtpmStateDir)),
		"--tpm2",
	}

	if res, err := r.providerConf.Exec.RunDetached(ctx, d, r.providerConf.Bash, cmdArgs...); err != nil {
		return fmt.Errorf("failed to start the swtpm process monitor: %w, diagnostics: %v", err, res.Diagnostics())
	}

	return nil
}

// waitSwTPMReady polls until the swtpm process is running and its control
// socket exists, failing with the tail of the swtpm log after
// swtpmStartTimeout.
func (r *SwTPM) waitSwTPMReady(ctx context.Context, d string) error {
	deadline := time.Now().Add(swtpmStartTimeout)
	for {
		if running, _, _ := readSwTPMPID(ctx, r.providerConf, d); running {
			if _, err := r.providerConf.Exec.Stat(ctx, filepath.Join(d, swtpmSocketFile)); err == nil {
				return nil
			}
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(swtpmPollInterval)
	}

	logTail := "(no log)"
	if x, err := r.providerConf.Exec.ReadFile(ctx, filepath.Join(d, swtpmLogFile)); err == nil && len(x) > 0 {
		logTail = tailString(string(x), swtpmLogTailBytes)
	}

	return fmt.Errorf("swtpm did not become ready within %s, log: %s", swtpmStartTimeout, logTail)
}

// stopSwTPM stops the process monitor and the swtpm process it supervises,
// making sure that neither survives.
func (r *SwTPM) stopSwTPM(ctx context.Context, d string) error {
	ex := r.providerConf.Exec

	// Graceful path: SIGTERM the monitor, whose signal handler forwards the
	// signal to swtpm, waits for it to exit and removes its own PID file. If
	// only an orphaned swtpm survives, SIGTERM it directly.
	pmRunning, pmPID, _ := readMonitorPID(ctx, r.providerConf, d)
	swRunning, swPID, _ := readSwTPMPID(ctx, r.providerConf, d)
	switch {
	case pmRunning:
		_ = ex.Kill(ctx, pmPID, syscall.SIGTERM)
	case swRunning:
		_ = ex.Kill(ctx, swPID, syscall.SIGTERM)
	default:
		return nil // Already stopped.
	}

	if r.waitSwTPMGone(ctx, d, swtpmStopTimeout) {
		return nil
	}

	// Escalate. SIGKILL cannot be trapped, so the monitor will never forward
	// it to its child: the monitor must die first (so that it cannot restart
	// swtpm) and then swtpm must be killed explicitly.
	if pmRunning, pmPID, _ = readMonitorPID(ctx, r.providerConf, d); pmRunning {
		if err := ex.Kill(ctx, pmPID, syscall.SIGKILL); err != nil {
			return fmt.Errorf("can't kill the swtpm process monitor (PID %d): %w", pmPID, err)
		}
	}
	if swRunning, swPID, _ = readSwTPMPID(ctx, r.providerConf, d); swRunning {
		if err := ex.Kill(ctx, swPID, syscall.SIGKILL); err != nil {
			return fmt.Errorf("can't kill the swtpm process (PID %d): %w", swPID, err)
		}
	}

	if !r.waitSwTPMGone(ctx, d, 2*time.Second) {
		return fmt.Errorf("the swtpm process and/or its process monitor are still running after SIGKILL")
	}

	return nil
}

// waitSwTPMGone polls until neither the process monitor nor the swtpm process
// is running anymore, giving up after timeout.
func (r *SwTPM) waitSwTPMGone(ctx context.Context, d string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		pmRunning, _, _ := readMonitorPID(ctx, r.providerConf, d)
		swRunning, _, _ := readSwTPMPID(ctx, r.providerConf, d)
		if !pmRunning && !swRunning {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(swtpmPollInterval)
	}
}

// readSwTPM reconciles the actual state of the swtpm process stack with the
// desired state in the model (self-healing, like the other daemon resources)
// and refreshes the model's computed attributes.
func (r *SwTPM) readSwTPM(ctx context.Context, d string, data *SwTPMModel) error {
	if _, err := r.providerConf.Exec.Stat(ctx, d); exec.IsNotExist(err) {
		return fmt.Errorf("resource directory does not exist")
	}

	// Determine desired state (default to "running" if not set).
	desiredState := "running"
	if !data.State.IsNull() && data.State.ValueString() != "" {
		desiredState = data.State.ValueString()
	}

	// The monitor being alive is what guarantees that swtpm itself is (or
	// shortly will be) running: swtpm exits every time an attached VM shuts
	// down and the monitor is what restarts it.
	pmRunning, _, _ := readMonitorPID(ctx, r.providerConf, d)
	actualState := "stopped"
	if pmRunning {
		actualState = "running"
	}

	if desiredState == "running" && actualState == "stopped" {
		tflog.Info(ctx, "swtpm is stopped but should be running, restarting...")
		if err := r.startSwTPM(ctx, d); err != nil {
			return fmt.Errorf("failed to start swtpm: %w", err)
		}
		if err := r.waitSwTPMReady(ctx, d); err != nil {
			return err
		}
		actualState = "running"
	} else if desiredState == "stopped" && actualState == "running" {
		tflog.Info(ctx, "swtpm is running but should be stopped, stopping...")
		if err := r.stopSwTPM(ctx, d); err != nil {
			return fmt.Errorf("failed to stop swtpm: %w", err)
		}
		actualState = "stopped"
	}

	data.State = types.StringValue(actualState)
	data.Socket = types.StringValue(filepath.Join(d, swtpmSocketFile))

	return nil
}

// readMonitorPID reports whether the process monitor of this resource is
// running. The PID file is written by the monitor script itself and removed
// on its clean exit. NOTE: the monitor is a bash process, so an executable
// check (IsRunning matches expectedExe against /proc/<pid>/exe) can never
// match the script path; like the other daemon resources this is a PID
// liveness check only.
func readMonitorPID(ctx context.Context, pConf *ZedAmigoProviderConfig, path string) (bool, int, error) {
	pidPath := filepath.Join(path, swtpmMonitorPIDFile)
	x, err := pConf.Exec.ReadFile(ctx, pidPath)
	if err != nil {
		return false, 0, fmt.Errorf("%w", err)
	}

	pid, err := strconv.ParseInt(string(bytes.TrimSpace(x)), 10, 32)
	if err != nil {
		return false, 0, fmt.Errorf("%w", err)
	}

	running, err := pConf.Exec.IsRunning(ctx, int(pid), "")
	if err != nil {
		return false, int(pid), fmt.Errorf("%w", err)
	}

	return running, int(pid), nil
}

// readSwTPMPID reports whether the swtpm process of this resource is running.
// The PID file is written by swtpm itself and removed on its clean exit.
func readSwTPMPID(ctx context.Context, pConf *ZedAmigoProviderConfig, path string) (bool, int, error) {
	pidPath := filepath.Join(path, swtpmPIDFile)
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
