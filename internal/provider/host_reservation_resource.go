// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	_ "embed"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/cmd/result"
)

const (
	hostReservationsDir     = "host_reservations"
	defaultReservationsPath = "/var/lib/zedamigo/reservations"
	// hostReservationArg0 is the conventional $0 passed to
	// `bash -c <script> <arg0> <mode> ...`; it only shows up in process
	// listings and log lines.
	hostReservationArg0 = "za-host-reservation"
)

//go:embed host_reservation.bash
var hostReservationScript string

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                = &HostReservation{}
	_ resource.ResourceWithImportState = &HostReservation{}
)

func NewHostReservation() resource.Resource {
	return &HostReservation{}
}

// HostReservation defines the resource implementation.
type HostReservation struct {
	providerConf *ZedAmigoProviderConfig
}

// HostReservationModel describes the resource data model.
type HostReservationModel struct {
	ID           types.String `tfsdk:"id"`
	Path         types.String `tfsdk:"path"`
	CPUs         types.Int64  `tfsdk:"cpus"`
	Mem          types.Int64  `tfsdk:"mem"`
	Devs         types.List   `tfsdk:"devs"`
	CPUsReserved types.List   `tfsdk:"cpus_reserved"`
	MemReserved  types.List   `tfsdk:"mem_reserved"`
	DevsReserved types.List   `tfsdk:"devs_reserved"`

	CPUsReservedCount  types.Int64 `tfsdk:"cpus_reserved_count"`
	MemReservedTotalGB types.Int64 `tfsdk:"mem_reserved_total_gb"`
}

func (r *HostReservation) getResourceDir(id string) string {
	return filepath.Join(r.providerConf.LibPath, hostReservationsDir, id)
}

func (r *HostReservation) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_host_reservation"
}

func (r *HostReservation) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reserve finite host capacity (CPUs, memory, devices) to avoid oversubscription.",
		MarkdownDescription: "Reserve a slice of finite host capacity (CPUs, GB of memory, `/dev` devices) so " +
			"that multiple VMs/edge-nodes — even across independent Terraform configurations — do not " +
			"oversubscribe the host or use the same block device concurrently.\n\n" +
			"Capacity must be **declared by the operator** by pre-creating empty slot files on the target " +
			"under the reservations `path`; this resource only claims among existing files and never creates " +
			"capacity itself. A slot file that is empty is free; one that holds a reservation marker (a " +
			"tab-separated line whose first field is the reservation id) is taken.\n\n" +
			"```\n" +
			"<path>/cpus/unit/<coreID>   one empty file per reservable CPU (filename is the core ID)\n" +
			"<path>/ram/gb/<index>       one empty file per reservable GB\n" +
			"<path>/devs/<abs-dev-path>  e.g. /dev/sdb -> <path>/devs/dev/sdb\n" +
			"```\n\n" +
			"A taken slot holds a tab-separated marker recording who claimed it — the reservation id " +
			"(field 1), the local and remote username, the local and remote hostname, and the source " +
			"directory of the Terraform configuration — so an operator can tell which user holds a slot.\n\n" +
			"Requesting more CPUs/GB than are free, or a device with no capacity file, is an error. " +
			"Reservations are durable: they persist on the target (and across reboots) until the resource " +
			"is destroyed. Claims are made atomic with `flock`, so this resource requires a Linux target " +
			"with util-linux `flock` installed. The `path` directory must be writable by the provider's " +
			"execution identity (use a host-wide path with `use_sudo`/root, or override `path`).",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				Description:         "Reservation identifier",
				MarkdownDescription: "Reservation identifier.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"path": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Description: "Root directory on the target that holds the reservation slot files. " +
					"Default: `/var/lib/zedamigo/reservations`.",
				MarkdownDescription: "Root directory on the target that holds the reservation slot files. " +
					"Defaults to `/var/lib/zedamigo/reservations`. Should live on a local filesystem " +
					"(advisory `flock` can be unreliable over NFS).",
				Default: stringdefault.StaticString(defaultReservationsPath),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"cpus": schema.Int64Attribute{
				Optional:            true,
				Description:         "Number of CPUs to reserve.",
				MarkdownDescription: "Number of CPUs to reserve from the pool of files under `<path>/cpus/unit/`.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"mem": schema.Int64Attribute{
				Optional:            true,
				Description:         "GB of memory to reserve.",
				MarkdownDescription: "Amount of memory, in GB, to reserve from the pool of files under `<path>/ram/gb/`.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"devs": schema.ListAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "Absolute `/dev/...` device paths to reserve. Each must have a corresponding " +
					"pre-created capacity file at `<path>/devs<dev>`.",
				MarkdownDescription: "List of absolute `/dev/...` device paths to reserve. Each entry must have a " +
					"corresponding operator-created capacity file at `<path>/devs<dev>` (e.g. `/dev/sdb` -> " +
					"`<path>/devs/dev/sdb`). Prefer stable names such as `/dev/disk/by-id/...`.",
				PlanModifiers: []planmodifier.List{
					listplanmodifier.RequiresReplace(),
				},
			},
			"cpus_reserved": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.Int64Type,
				Description:         "The CPU core IDs that were actually reserved (e.g. `[1, 2, 6, 7]`).",
				MarkdownDescription: "The CPU core IDs that were actually reserved, e.g. `[1, 2, 6, 7]`. Useful as input to an edge node's `cpu_pins`.",
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
			},
			"mem_reserved": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.Int64Type,
				Description:         "The GB slot indices that were actually reserved.",
				MarkdownDescription: "The GB slot indices that were actually reserved.",
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
			},
			"devs_reserved": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				Description:         "The device paths that were actually reserved.",
				MarkdownDescription: "The device paths that were actually reserved (equal to `devs` on success).",
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
			},
			"cpus_reserved_count": schema.Int64Attribute{
				Computed:            true,
				Description:         "Number of CPUs that were actually reserved (length of cpus_reserved).",
				MarkdownDescription: "Number of CPUs that were actually reserved (the length of `cpus_reserved`).",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"mem_reserved_total_gb": schema.Int64Attribute{
				Computed:            true,
				Description:         "Total memory actually reserved, in GB (length of mem_reserved).",
				MarkdownDescription: "Total memory actually reserved, in GB. Each `mem_reserved` slot is 1 GB, so this equals the length of `mem_reserved`.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *HostReservation) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
}

// reservedRoot returns the reservations root directory for this resource,
// falling back to the default when path is null/unknown (e.g. on import).
func (r *HostReservation) reservedRoot(data *HostReservationModel) string {
	if !data.Path.IsNull() && !data.Path.IsUnknown() && data.Path.ValueString() != "" {
		return data.Path.ValueString()
	}
	return defaultReservationsPath
}

// localReservationFields gathers the controller-side ("local") identity stored
// in a reservation marker: the local username, the local hostname and the source
// directory of the Terraform configuration driving the apply (the same value
// createTFBackPointer records). The script resolves the target-side ("remote")
// username/hostname itself; for a localhost target the two sides coincide, while
// for a remote target these identify who made the claim, and from where. Every
// field is sanitized so none can break the tab-separated single-line marker.
func localReservationFields() (localUser, localHost, configDir string) {
	if u, err := user.Current(); err == nil && u.Username != "" {
		localUser = u.Username
	} else if v := os.Getenv("USER"); v != "" {
		localUser = v
	} else {
		localUser = os.Getenv("LOGNAME")
	}
	if h, err := os.Hostname(); err == nil {
		localHost = h
	}
	if wd, err := os.Getwd(); err == nil {
		configDir = wd
	}
	return sanitizeReservationField(localUser), sanitizeReservationField(localHost), sanitizeReservationField(configDir)
}

// sanitizeReservationField squashes tabs/newlines to spaces so a value can be
// stored as one field of the tab-separated, single-line reservation marker
// without making the id (field 1) ambiguous. It mirrors the shell `sanitize`.
func sanitizeReservationField(s string) string {
	return strings.NewReplacer("\t", " ", "\n", " ", "\r", " ").Replace(s)
}

// runScript invokes the embedded host_reservation.bash on the target as a single
// argv (so the whole critical section runs under one flock in one call).
func (r *HostReservation) runScript(ctx context.Context, logDir string, args ...string) (result.Result, error) {
	full := make([]string, 0, len(args)+3)
	full = append(full, "-c", hostReservationScript, hostReservationArg0)
	full = append(full, args...)
	return r.providerConf.Exec.Run(ctx, logDir, r.providerConf.Bash, full...)
}

// scriptErrDetail builds a concise diagnostic detail for a failed
// host_reservation.bash run. The script prints an actionable message to stderr
// on every failure path, so that is preferred; the exec error (or bare exit
// code) is only a fallback for the rare case where stderr is empty (e.g. a
// transport error before the script produced output). It deliberately omits
// res.Diagnostics(), whose argv dump carries the entire embedded script and is
// pure noise for this resource.
func scriptErrDetail(res result.Result, err error) string {
	if msg := strings.TrimSpace(res.Stderr); msg != "" {
		return msg
	}
	if err != nil {
		return err.Error()
	}
	return fmt.Sprintf("exit code %d", res.ExitCode)
}

func (r *HostReservation) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data HostReservationModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if r.providerConf.Flock == "" {
		resp.Diagnostics.AddError("Host Reservation Resource Error",
			"The `flock` executable was not found on the target. The zedamigo_host_reservation resource "+
				"requires a Linux target with util-linux `flock` installed.")
		return
	}

	devs, diags := cleanValidateDevs(ctx, data.Devs)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	ncpu := int64OrZero(data.CPUs)
	nmem := int64OrZero(data.Mem)
	if ncpu < 0 || nmem < 0 {
		resp.Diagnostics.AddError("Host Reservation Resource Error", "cpus and mem must be >= 0.")
		return
	}
	if ncpu == 0 && nmem == 0 && len(devs) == 0 {
		resp.Diagnostics.AddError("Host Reservation Resource Error",
			"At least one of `cpus`, `mem`, or `devs` must be set to a non-zero/non-empty value.")
		return
	}

	id, err := newResourceID()
	if err != nil {
		resp.Diagnostics.AddError("Host Reservation Resource Error",
			fmt.Sprintf("Unable to generate a new resource ID: %s", err))
		return
	}
	data.ID = types.StringValue(id)

	root := r.reservedRoot(&data)
	data.Path = types.StringValue(root)

	d := r.getResourceDir(id)
	if err := r.providerConf.Exec.MkdirAll(ctx, d, 0o700); err != nil {
		resp.Diagnostics.AddError("Host Reservation Resource Error",
			fmt.Sprintf("Unable to create resource specific directory: %s", err))
		return
	}
	if err := createTFBackPointer(ctx, r.providerConf.Exec, d); err != nil {
		resp.Diagnostics.AddError("Host Reservation Resource Error",
			fmt.Sprintf("Unable to create resource specific file: %s", err))
		return
	}

	localUser, localHost, configDir := localReservationFields()
	args := []string{
		"reserve", r.providerConf.Flock, id, root,
		strconv.FormatInt(ncpu, 10), strconv.FormatInt(nmem, 10),
		localUser, localHost, configDir,
	}
	args = append(args, devs...)
	res, err := r.runScript(ctx, d, args...)
	if err != nil {
		// The script validates everything before writing and rolls back on any
		// failure, so no host markers remain. Drop the orphan bookkeeping dir
		// and surface the actionable message (script stderr).
		_ = r.providerConf.Exec.Remove(ctx, d)
		resp.Diagnostics.AddError("Host Reservation claim failed", scriptErrDetail(res, err))
		return
	}

	rr, err := parseReservedJSON(res.Stdout)
	if err != nil {
		// Defensive: script claimed success but output was unparseable. Release
		// whatever it may have written, then fail.
		_, _ = r.runScript(ctx, d, "release", r.providerConf.Flock, id, root)
		_ = r.providerConf.Exec.Remove(ctx, d)
		resp.Diagnostics.AddError("Host Reservation Resource Error", err.Error())
		return
	}

	resp.Diagnostics.Append(applyReserved(&data, rr)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "Host Reservation created successfully")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *HostReservation) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data HostReservationModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := data.ID.ValueString()
	root := r.reservedRoot(&data)
	d := r.getResourceDir(id)

	res, err := r.runScript(ctx, d, "scan", "", id, root)
	if err != nil {
		resp.Diagnostics.AddError("Host Reservation scan failed", scriptErrDetail(res, err))
		return
	}
	rr, err := parseReservedJSON(res.Stdout)
	if err != nil {
		resp.Diagnostics.AddError("Host Reservation Resource Error", err.Error())
		return
	}

	// A live reservation always owns at least one marker (Create requires it).
	// Owning nothing means the markers are gone (tree wiped / released
	// externally): drop from state so a subsequent apply re-claims.
	if len(rr.CPUs) == 0 && len(rr.Mem) == 0 && len(rr.Devs) == 0 {
		resp.State.RemoveResource(ctx)
		return
	}

	data.Path = types.StringValue(root)
	resp.Diagnostics.Append(applyReserved(&data, rr)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *HostReservation) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Host Reservation Resource Update Error",
		"Update is not supported; every input attribute forces replacement.")
}

func (r *HostReservation) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data HostReservationModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := data.ID.ValueString()
	root := r.reservedRoot(&data)
	d := r.getResourceDir(id)

	// Release is best-effort and idempotent; it truncates every marker owned by
	// this id back to empty. A lock error must not wedge a destroy.
	res, err := r.runScript(ctx, d, "release", r.providerConf.Flock, id, root)
	if err != nil {
		resp.Diagnostics.AddWarning("Host Reservation release reported an error",
			scriptErrDetail(res, err))
	}

	if err := r.providerConf.Exec.Remove(ctx, d); err != nil {
		resp.Diagnostics.AddError("Host Reservation Resource Delete Error",
			fmt.Sprintf("Can't delete resource directory: %v", err))
		return
	}
}

func (r *HostReservation) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Accept "<id>" or "<id>:<path>" so a non-default reservations path can be
	// carried in (import Read runs against state, with no config to merge). A
	// reservation id is 8 hex chars, so ':' is an unambiguous separator.
	idPart := req.ID
	if i := strings.IndexByte(req.ID, ':'); i >= 0 {
		idPart = req.ID[:i]
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("path"), req.ID[i+1:])...)
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), idPart)...)
}

// reservedResult is the JSON emitted by host_reservation.bash.
type reservedResult struct {
	CPUs []int64  `json:"cpus"`
	Mem  []int64  `json:"mem"`
	Devs []string `json:"devs"`
}

func parseReservedJSON(stdout string) (reservedResult, error) {
	var rr reservedResult
	s := strings.TrimSpace(stdout)
	if s == "" {
		return rr, fmt.Errorf("empty output from the reservation script")
	}
	if err := json.Unmarshal([]byte(s), &rr); err != nil {
		return rr, fmt.Errorf("can't parse reservation script output %q: %w", s, err)
	}
	return rr, nil
}

// applyReserved sets the computed *_reserved lists on the model from rr. The
// lists are always non-null (possibly empty) so Terraform never sees a null
// computed value.
func applyReserved(data *HostReservationModel, rr reservedResult) diag.Diagnostics {
	var diags diag.Diagnostics
	cpus, d := buildInt64List(rr.CPUs)
	diags.Append(d...)
	mem, d := buildInt64List(rr.Mem)
	diags.Append(d...)
	devs, d := buildStringList(rr.Devs)
	diags.Append(d...)
	data.CPUsReserved = cpus
	data.MemReserved = mem
	data.DevsReserved = devs
	data.CPUsReservedCount = types.Int64Value(int64(len(rr.CPUs)))
	data.MemReservedTotalGB = types.Int64Value(int64(len(rr.Mem)))
	return diags
}

func buildInt64List(xs []int64) (types.List, diag.Diagnostics) {
	vals := make([]attr.Value, len(xs))
	for i, x := range xs {
		vals[i] = types.Int64Value(x)
	}
	return types.ListValue(types.Int64Type, vals)
}

func buildStringList(ss []string) (types.List, diag.Diagnostics) {
	vals := make([]attr.Value, len(ss))
	for i, s := range ss {
		vals[i] = types.StringValue(s)
	}
	return types.ListValue(types.StringType, vals)
}

func int64OrZero(v types.Int64) int64 {
	if v.IsNull() || v.IsUnknown() {
		return 0
	}
	return v.ValueInt64()
}

// cleanValidateDevs decodes and validates the devs input into clean absolute
// /dev/... paths. Validation is pure string-shape (no host access): it rejects
// empty entries, whitespace, "..", non-clean paths, non-/dev/ paths and
// duplicates so the shell mapping "<root>/devs<dev>" stays bounded and the
// line/JSON handling unambiguous.
func cleanValidateDevs(ctx context.Context, list types.List) ([]string, diag.Diagnostics) {
	var diags diag.Diagnostics
	if list.IsNull() || list.IsUnknown() {
		return nil, diags
	}

	var raw []string
	diags.Append(list.ElementsAs(ctx, &raw, false)...)
	if diags.HasError() {
		return nil, diags
	}

	seen := make(map[string]bool, len(raw))
	out := make([]string, 0, len(raw))
	for _, d := range raw {
		switch {
		case d == "":
			diags.AddError("Invalid device entry", "A device entry must not be empty.")
		case strings.ContainsAny(d, " \t\n\r\v\f"):
			diags.AddError("Invalid device entry", fmt.Sprintf("Device entry %q must not contain whitespace.", d))
		case strings.Contains(d, ".."):
			diags.AddError("Invalid device entry", fmt.Sprintf("Device entry %q must not contain '..'.", d))
		case filepath.Clean(d) != d:
			diags.AddError("Invalid device entry", fmt.Sprintf("Device entry %q is not a clean path; did you mean %q?", d, filepath.Clean(d)))
		case !strings.HasPrefix(d, "/dev/") || len(d) <= len("/dev/"):
			diags.AddError("Invalid device entry", fmt.Sprintf("Device entry %q must be an absolute path under /dev/.", d))
		case seen[d]:
			diags.AddError("Invalid device entry", fmt.Sprintf("Device entry %q is listed more than once.", d))
		default:
			seen[d] = true
			out = append(out, d)
		}
	}
	return out, diags
}
