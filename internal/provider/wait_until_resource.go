// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/cmd/result"
	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/exec"
	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/undent"
)

const (
	waitUntilDir             = "wait_until"
	waitUntilScriptName      = "wait_until.sh"
	waitUntilSummaryName     = "summary.txt"
	waitUntilAttemptsDirName = "attempts"
	waitUntilDefaultTimeout  = "30m"
	waitUntilDefaultInterval = "15s"
	// waitUntilRetainLogs is how many of the most recent COMPLETED attempt log
	// directories are kept; the first attempt's directory is always kept on top
	// of those. Each Exec.Run writes 3 log files, so the default 30m/15s (~120
	// attempts) would otherwise leave ~360 files behind.
	waitUntilRetainLogs = 5
	// waitUntilOutputTailBytes caps how much of the script's output is kept in
	// Terraform state and shown in diagnostics.
	waitUntilOutputTailBytes = 8192
	// waitUntilLogTailBytes caps the stderr excerpt included in the per-attempt
	// tflog line.
	waitUntilLogTailBytes = 512
)

// errWaitTimeout is returned by waitForScript when the overall time budget is
// exhausted without the script ever exiting 0.
var errWaitTimeout = errors.New("the script did not succeed before the timeout")

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                = &WaitUntil{}
	_ resource.ResourceWithImportState = &WaitUntil{}
)

func NewWaitUntil() resource.Resource {
	return &WaitUntil{}
}

// WaitUntil defines the resource implementation.
type WaitUntil struct {
	providerConf *ZedAmigoProviderConfig
}

// WaitUntilModel describes the resource data model.
type WaitUntilModel struct {
	ID             types.String `tfsdk:"id"`
	Script         types.String `tfsdk:"script"`
	Interpreter    types.List   `tfsdk:"interpreter"`
	Timeout        types.String `tfsdk:"timeout"`
	Interval       types.String `tfsdk:"interval"`
	AttemptTimeout types.String `tfsdk:"attempt_timeout"`
	Triggers       types.Map    `tfsdk:"triggers"`

	Attempts   types.Int64  `tfsdk:"attempts"`
	Elapsed    types.String `tfsdk:"elapsed"`
	Stdout     types.String `tfsdk:"stdout"`
	Stderr     types.String `tfsdk:"stderr"`
	ScriptPath types.String `tfsdk:"script_path"`
}

func (r *WaitUntil) getResourceDir(id string) string {
	return filepath.Join(r.providerConf.LibPath, waitUntilDir, id)
}

func (r *WaitUntil) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_wait_until"
}

func (r *WaitUntil) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Barrier: repeatedly run a script on the target until it exits 0.",
		MarkdownDescription: undent.Md(`
		Run a script ON THE PROVIDER |target| repeatedly until it exits |0|, or until |timeout| expires. The resource is
		created successfully only if the script succeeded; otherwise the apply fails.

		This is the provider-native replacement for a |null_resource| holding an inline |local-exec| / |remote-exec| poll
		loop. The script goes through the same execution path as every other zedamigo operation, so the identical
		configuration works whether |target| is |localhost| (the script runs locally) or a remote host (the script runs
		over SSH) — there is no |local-exec| vs |remote-exec| choice to get wrong. Everything the script references
		(ports forwarded by |zedamigo_edge_node|, files under |lib_path|, |docker|) is resolved on the target, which is
		where those things actually exist.

		The script must be an idempotent single-shot probe: exit |0| when the condition holds, non-zero when it does not.
		Do NOT put a retry loop inside the script — |interval| and |timeout| own the retrying, and an inner loop defeats
		|attempt_timeout| and the per-attempt logging.

		The script is written once to |<lib_path>/wait_until/<id>/wait_until.sh| (mode 0755) and re-executed by path on
		every attempt. Each attempt's stdout/stderr/command logs land in their own
		|<lib_path>/wait_until/<id>/attempts/NNNN/| directory on the target; the first attempt's logs and the most recent
		few are retained, the rest are pruned. A |summary.txt| next to the script records the outcome. When the wait
		fails the whole directory is deliberately left behind as the post-mortem record.

		NOTE: no PATH is injected, so a non-interactive remote shell may hand the script a minimal PATH. Use absolute
		paths, or set PATH at the top of the script.

		NOTE: if the script needs to |ssh| onward from a remote target (a common shape — probing an edge node on a
		port the target forwards), set |forward_agent = true| in the provider's |ssh| block instead of copying a
		private key onto the target.

		NOTE: a provider cannot stream output to the terminal the way |local-exec| does. Run with |TF_LOG=info| to see
		per-attempt progress; full per-attempt output stays on the target.`),

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				Description:         "Wait-until resource identifier",
				MarkdownDescription: "Wait-until resource identifier.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"script": schema.StringAttribute{
				Required: true,
				Description: "Script content run on the target. Exit 0 means the condition is met; any non-zero exit " +
					"means \"not ready yet\".",
				MarkdownDescription: undent.Md(`
				The script content, run on the target by |interpreter|. Exit |0| means the condition is met and the wait
				ends; any non-zero exit means "not ready yet" and the script is re-run after |interval|. Changing the
				script replaces the resource, i.e. re-runs the barrier.`),
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"interpreter": schema.ListAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "Interpreter argv prefix. Defaults to the `bash` path the provider resolved on the " +
					"target. An empty list executes the script file directly, relying on its shebang.",
				MarkdownDescription: undent.Md(`
				The interpreter and its arguments, e.g. |["/bin/sh", "-e"]|. The script's path on the target is appended
				as the final argument. If omitted it defaults to the |bash| path the provider already resolved on the
				target — this cannot be a schema default because defaults are evaluated during plan, where the provider
				configuration is not yet available.

				An explicitly empty list (|[]|) executes the script file directly, which honors a |#!| shebang. When an
				interpreter IS given, a shebang line is just a comment.

				NOTE: a shebang-less script executed directly fails with "exec format error" on a |localhost| target but
				is silently run by |sh| on a remote target. Always include a shebang when using |[]|.`),
				PlanModifiers: []planmodifier.List{
					listplanmodifier.RequiresReplace(),
				},
			},
			"timeout": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Overall time budget for the wait (Go duration string). Default: `30m`.",
				MarkdownDescription: undent.Md(`
				Overall time budget for the wait, as a Go duration string (e.g. |30m|, |1h30m|). Default: |30m|. Changing
				it does NOT re-run a barrier that already succeeded.`),
				Default: stringdefault.StaticString(waitUntilDefaultTimeout),
				Validators: []validator.String{
					positiveDurationValidator{},
				},
			},
			"interval": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Delay between two attempts (Go duration string). Default: `15s`.",
				MarkdownDescription: undent.Md(`
				Delay between two attempts, as a Go duration string. The first attempt runs immediately. Default: |15s|.`),
				Default: stringdefault.StaticString(waitUntilDefaultInterval),
				Validators: []validator.String{
					positiveDurationValidator{},
				},
			},
			"attempt_timeout": schema.StringAttribute{
				Optional: true,
				Description: "Optional per-attempt time budget (Go duration string). Unset means an attempt is " +
					"bounded only by the remaining overall `timeout`.",
				MarkdownDescription: undent.Md(`
				Optional per-attempt time budget, as a Go duration string. When unset a single attempt is bounded only by
				the remaining overall |timeout|, so one hung attempt can consume the entire budget.

				WARNING: abandoning an attempt does NOT kill it — the process keeps running on the target until it exits
				by itself. Prefer making the probe self-bounding (|timeout 10 ...|, |ssh -o ConnectTimeout=5|) and treat
				|attempt_timeout| as a backstop.`),
				Validators: []validator.String{
					positiveDurationValidator{},
				},
			},
			"triggers": schema.MapAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "Arbitrary map of values that, when changed, re-runs the barrier (`null_resource.triggers` semantics).",
				MarkdownDescription: undent.Md(`
				Arbitrary map of values whose change forces the barrier to be re-run, exactly like |null_resource|'s
				|triggers|. Typically the |id| of each resource whose readiness is being awaited, which also establishes
				the dependency that orders this resource after them.`),
				PlanModifiers: []planmodifier.Map{
					mapplanmodifier.RequiresReplace(),
				},
			},

			// --- recorded outcome ---

			"attempts": schema.Int64Attribute{
				Computed:            true,
				Description:         "Number of times the script was run before it succeeded.",
				MarkdownDescription: "Number of times the script was run before it succeeded.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"elapsed": schema.StringAttribute{
				Computed:            true,
				Description:         "How long the wait took, as a Go duration string rounded to the second.",
				MarkdownDescription: "How long the wait took, as a Go duration string rounded to the second.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"stdout": schema.StringAttribute{
				Computed:            true,
				Description:         "Stdout of the successful attempt (last 8 KiB).",
				MarkdownDescription: "Stdout of the successful attempt, truncated to the last 8 KiB. Useful when the probe also produces a value to be consumed downstream.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"stderr": schema.StringAttribute{
				Computed:            true,
				Description:         "Stderr of the successful attempt (last 8 KiB).",
				MarkdownDescription: "Stderr of the successful attempt, truncated to the last 8 KiB.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"script_path": schema.StringAttribute{
				Computed:            true,
				Description:         "Path of the script file on the target.",
				MarkdownDescription: "Path of the script file on the target.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *WaitUntil) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// probeKind classifies the outcome of a single attempt.
type probeKind int

const (
	probeReady     probeKind = iota // exited 0: the condition is met
	probeNotReady                   // ran and exited non-zero: "not yet"
	probeExecError                  // could not be run at all
	probeHung                       // abandoned: did not finish within its budget
)

func (k probeKind) String() string {
	switch k {
	case probeReady:
		return "ready"
	case probeNotReady:
		return "not-ready"
	case probeExecError:
		return "exec-error"
	case probeHung:
		return "abandoned"
	}
	return "unknown"
}

// classifyProbe maps a Result onto a probeKind. Both executors report a non-zero
// exit as a NON-NIL Error with ExitCode set, so the error alone cannot tell "the
// probe says not-ready" from "the probe could not be run"; only ExitCode can.
// Note that RunBG discards Run's error return, so Result.Error is the only
// signal available here.
//
// The ExitCode == 0 && Error != nil case is deliberately conservative and
// reported as an execution error: it covers real transport/spawn failures, and
// also a remotely signal-killed probe (golang.org/x/crypto/ssh reports the
// signal with exit status 0), which is not a meaningful "not ready" answer
// either. A locally signal-killed probe yields ExitCode -1 and lands in
// probeNotReady.
func classifyProbe(res result.Result) probeKind {
	switch {
	case res.Error == nil:
		return probeReady
	case res.ExitCode != 0:
		return probeNotReady
	default:
		return probeExecError
	}
}

// runProbe executes one attempt and returns as soon as it finishes, its budget
// expires, or ctx is cancelled. Neither LocalExecutor.Run nor SSHExecutor.Run
// honors ctx or has any per-command timeout (internal/cmd is not context-aware
// and ssh.go does not wire ctx into sess.Run), so RunBG + select is the only way
// to bound an attempt. RunBG's channel is buffered and closed by the producer, so
// abandoning it leaks neither goroutine nor channel — BUT the process it started
// KEEPS RUNNING ON THE TARGET until it exits by itself.
func runProbe(ctx context.Context, ex exec.Executor, logDir string, argv []string, budget time.Duration) (result.Result, probeKind, error) {
	ch := ex.RunBG(ctx, logDir, argv[0], argv[1:]...)

	t := time.NewTimer(budget)
	defer t.Stop()

	select {
	case res := <-ch:
		return res, classifyProbe(res), nil
	case <-t.C:
		return result.Result{Cmd: argv[0], Args: argv[1:], TimedOut: true}, probeHung, nil
	case <-ctx.Done():
		return result.Result{Cmd: argv[0], Args: argv[1:]}, probeHung, ctx.Err()
	}
}

// waitParams are the inputs of one wait; separated from the Terraform model so
// the loop can be unit tested against a plain executor.
type waitParams struct {
	AttemptsRoot   string
	Argv           []string
	Timeout        time.Duration
	Interval       time.Duration
	AttemptTimeout time.Duration // 0 => bounded only by the overall deadline
	RetainLogs     int           // most recent COMPLETED attempt dirs to keep; 0 => keep all
}

// waitOutcome is what actually happened, for state and for diagnostics.
type waitOutcome struct {
	Attempts int
	Elapsed  time.Duration
	Last     result.Result
	LastKind probeKind
	LastLogs string

	// LastCompleted is the most recent attempt that actually finished, i.e. was
	// not abandoned, and LastCompletedAttempt is its number (0 if there is none).
	//
	// These exist because an abandoned attempt carries no output at all, and the
	// attempt in flight when the overall deadline arrives is routinely abandoned:
	// its budget is whatever is left of the deadline, which is a uniformly random
	// slice of one interval. Without this the timeout diagnostic — the single most
	// important message this resource produces — would frequently carry no clue
	// about WHY the condition was never met.
	LastCompleted        result.Result
	LastCompletedAttempt int
}

// waitForScript runs p.Argv every p.Interval until it exits 0, and returns a nil
// error only in that case. Otherwise it returns errWaitTimeout, a context error,
// or an execution error.
func waitForScript(ctx context.Context, ex exec.Executor, p waitParams) (waitOutcome, error) {
	start := time.Now()
	deadline := start.Add(p.Timeout)

	var (
		out waitOutcome
		// completed holds the log dirs of attempts that FINISHED, so they are
		// safe to prune. An abandoned attempt may still be writing into its own.
		completed []string
	)

	for attempt := 1; ; attempt++ {
		budget := time.Until(deadline)
		if budget <= 0 {
			out.Elapsed = time.Since(start)
			return out, errWaitTimeout
		}
		if p.AttemptTimeout > 0 && p.AttemptTimeout < budget {
			budget = p.AttemptTimeout
		}

		// One log dir per attempt: it keeps the resource dir tidy, makes pruning a
		// single Remove, and avoids the collision a shared dir would have — log
		// file names only carry second resolution, so two attempts landing in the
		// same second would overwrite each other's logs.
		logDir := filepath.Join(p.AttemptsRoot, fmt.Sprintf("%04d", attempt))

		res, kind, err := runProbe(ctx, ex, logDir, p.Argv, budget)
		out.Attempts, out.Elapsed = attempt, time.Since(start)
		out.Last, out.LastKind, out.LastLogs = res, kind, logDir
		if kind != probeHung {
			out.LastCompleted, out.LastCompletedAttempt = res, attempt
		}
		if err != nil { // ctx cancelled: SIGINT, or the apply was aborted
			return out, err
		}

		switch kind {
		case probeReady:
			tflog.Info(ctx, "wait_until: condition met", map[string]any{
				"attempt": attempt,
				"elapsed": out.Elapsed.Round(time.Second).String(),
				"logs":    logDir,
			})
			return out, nil

		case probeExecError:
			// The interpreter path and the script file are both provider-managed,
			// so failing to even RUN them on the very first attempt is a
			// configuration/connectivity error, not "not ready yet". Fail now
			// instead of burning the whole timeout on it.
			//
			// NOTE: this only catches what the executor itself could not start. A
			// remote target reaches the script through a shell, which reports a
			// missing interpreter as exit 127 — indistinguishable from a probe
			// saying "not ready" — so on a remote target that particular mistake
			// is retried until the timeout rather than failing fast.
			if attempt == 1 {
				return out, fmt.Errorf("the script could not be run on the target: %w", res.Error)
			}
			tflog.Warn(ctx, "wait_until: the probe could not be run", map[string]any{
				"attempt": attempt,
				"error":   res.Error.Error(),
			})

		case probeHung:
			tflog.Warn(ctx, "wait_until: probe abandoned, it exceeded its budget (the process may still be running on the target)",
				map[string]any{
					"attempt": attempt,
					"budget":  budget.String(),
					"logs":    logDir,
				})

		default: // probeNotReady
			tflog.Info(ctx, "wait_until: not ready yet", map[string]any{
				"attempt":   attempt,
				"exit_code": res.ExitCode,
				"elapsed":   out.Elapsed.Round(time.Second).String(),
				"remaining": time.Until(deadline).Round(time.Second).String(),
				"stderr":    tailString(res.Stderr, waitUntilLogTailBytes),
			})
		}

		if kind != probeHung {
			completed = pruneAttemptLogs(ctx, ex, append(completed, logDir), p.RetainLogs)
		}

		remaining := time.Until(deadline)
		if remaining <= 0 {
			out.Elapsed = time.Since(start)
			return out, errWaitTimeout
		}
		// Never overshoot the deadline just to complete a full interval.
		sleep := p.Interval
		if remaining < sleep {
			sleep = remaining
		}

		timer := time.NewTimer(sleep)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			out.Elapsed = time.Since(start)
			return out, ctx.Err()
		}
		timer.Stop()
	}
}

// pruneAttemptLogs removes the oldest COMPLETED attempt log dirs so that at most
// `keep` of them survive, always sparing the first attempt — whose output is
// usually the most diagnostic (a missing tool, a bad shebang). It returns the
// retained slice. Pruning failures are logged, never fatal: the wait itself is
// what matters.
func pruneAttemptLogs(ctx context.Context, ex exec.Executor, completed []string, keep int) []string {
	if keep <= 0 || len(completed) <= keep+1 {
		return completed
	}

	victim := completed[1] // completed[0] is attempt 1: always kept
	if err := ex.Remove(ctx, victim); err != nil {
		tflog.Debug(ctx, "wait_until: can't prune an attempt log dir", map[string]any{
			"dir":   victim,
			"error": err.Error(),
		})
	}

	return append(completed[:1], completed[2:]...)
}

// waitUntilArgv builds one attempt's argv: the interpreter (possibly empty, to
// execute the script directly via its shebang) followed by the script path. It
// copies rather than appending in place so a slice returned by ElementsAs — which
// may have spare capacity — is never aliased.
func waitUntilArgv(interp []string, scriptPath string) []string {
	argv := make([]string, 0, len(interp)+1)
	argv = append(argv, interp...)

	return append(argv, scriptPath)
}

// resolveInterpreter decodes the interpreter list. A null/absent list defaults to
// the bash path the provider resolved on the target: a schema Default cannot be
// used because defaults are evaluated during plan, where the provider config
// (and therefore the target's bash path) is not available. An explicitly EMPTY
// list means "execute the script file directly" and relies on its shebang.
func (r *WaitUntil) resolveInterpreter(ctx context.Context, l types.List) ([]string, diag.Diagnostics) {
	var dz diag.Diagnostics

	if l.IsNull() || l.IsUnknown() {
		if r.providerConf.Bash == "" {
			dz.AddError("Wait Until Resource Error",
				"The `bash` executable was not found on the target, so the default interpreter cannot be used. "+
					"Install bash on the target or set an explicit `interpreter`.")
			return nil, dz
		}
		return []string{r.providerConf.Bash}, dz
	}

	var interp []string
	dz.Append(l.ElementsAs(ctx, &interp, false)...)
	if dz.HasError() {
		return nil, dz
	}

	return interp, dz
}

// waitUntilDurations parses the three duration attributes. The schema validator
// already rejects malformed values at plan time, so a failure here means the
// state was hand-edited or written by an older provider version.
func waitUntilDurations(data *WaitUntilModel) (timeout, interval, attemptTimeout time.Duration, dz diag.Diagnostics) {
	parse := func(name string, v types.String, fallback string) time.Duration {
		s := fallback
		if !v.IsNull() && !v.IsUnknown() {
			s = v.ValueString()
		}
		if s == "" {
			return 0
		}

		d, err := time.ParseDuration(s)
		if err != nil {
			dz.AddError("Wait Until Resource Error",
				fmt.Sprintf("Can't parse `%s` as a duration: %s", name, err))
			return 0
		}
		if d <= 0 {
			dz.AddError("Wait Until Resource Error",
				fmt.Sprintf("`%s` must be a positive duration, got %s.", name, d))
			return 0
		}

		return d
	}

	timeout = parse("timeout", data.Timeout, waitUntilDefaultTimeout)
	interval = parse("interval", data.Interval, waitUntilDefaultInterval)
	// attempt_timeout is optional with no default: unset means "bounded only by
	// the remaining overall timeout".
	attemptTimeout = parse("attempt_timeout", data.AttemptTimeout, "")

	return timeout, interval, attemptTimeout, dz
}

// tailString keeps the last n bytes of s, marking the elision. The tail is kept
// because a failing probe's useful output is at the end. It may cut mid-rune;
// that is acceptable for a diagnostic.
func tailString(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}

	return "...[truncated]...\n" + s[len(s)-n:]
}

// indentLines prefixes every line of s with two spaces so a script's output is
// visually separated from the diagnostic's own prose.
func indentLines(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i := range lines {
		lines[i] = "  " + lines[i]
	}

	return strings.Join(lines, "\n")
}

// waitUntilErrDiag builds the single diagnostic for a failed wait. It
// deliberately does not use res.Diagnostics(), which emits three separate errors
// (one per stream), and it follows scriptErrDetail's principle of leading with
// the script's own output.
func waitUntilErrDiag(o waitOutcome, p waitParams, resourceDir string, err error) (summary, detail string) {
	var b strings.Builder

	cancelled := errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)

	switch {
	case errors.Is(err, errWaitTimeout):
		summary = "Timed out waiting for the condition"
		fmt.Fprintf(&b, "The script did not exit 0 within %s.\n", p.Timeout)
	case cancelled:
		summary = "Waiting for the condition was aborted"
		fmt.Fprintf(&b, "The wait was cancelled after %s: %s\n", o.Elapsed.Round(time.Second), err)
	default:
		summary = "The script could not be run on the target"
		fmt.Fprintf(&b, "%s\n", err)
	}

	fmt.Fprintf(&b, "attempts: %d, elapsed: %s, interval: %s\n",
		o.Attempts, o.Elapsed.Round(time.Second), p.Interval)

	// shown is the attempt whose output is worth reporting. An abandoned attempt
	// has none, so fall back to the most recent one that finished.
	shown := o.Last
	switch o.LastKind {
	case probeHung:
		// A cancellation abandons the in-flight attempt too, but blaming its
		// budget for that would be wrong.
		if cancelled {
			fmt.Fprintf(&b, "last attempt: abandoned mid-run by the cancellation "+
				"(the process may still be running on the target)\n")
		} else {
			fmt.Fprintf(&b, "last attempt: abandoned, it did not finish within its budget "+
				"(the process may still be running on the target)\n")
		}
		if o.LastCompletedAttempt > 0 {
			shown = o.LastCompleted
			fmt.Fprintf(&b, "last completed attempt (#%d) exit code: %d\n",
				o.LastCompletedAttempt, o.LastCompleted.ExitCode)
		}
	case probeExecError:
		fmt.Fprintf(&b, "last attempt: could not be run: %s\n", o.Last.Error)
	default:
		fmt.Fprintf(&b, "last attempt exit code: %d\n", o.Last.ExitCode)
	}

	if s := strings.TrimSpace(shown.Stderr); s != "" {
		fmt.Fprintf(&b, "\nlast stderr:\n%s\n", indentLines(tailString(s, waitUntilOutputTailBytes)))
	}
	if s := strings.TrimSpace(shown.Stdout); s != "" {
		fmt.Fprintf(&b, "\nlast stdout:\n%s\n", indentLines(tailString(s, waitUntilOutputTailBytes)))
	}

	fmt.Fprintf(&b, "\nThe script and the per-attempt output logs were kept on the target under:\n  %s\n", resourceDir)

	return summary, b.String()
}

// writeSummary records the outcome next to the script on the target. When the
// wait fails no state is written, so the resource directory is the only surviving
// evidence and needs to be self-describing. Failing to write it is not fatal.
func (r *WaitUntil) writeSummary(ctx context.Context, dir string, p waitParams, o waitOutcome, waitErr error) {
	var b strings.Builder

	outcome := "succeeded"
	switch {
	case waitErr == nil:
	case errors.Is(waitErr, errWaitTimeout):
		outcome = "timed out"
	case errors.Is(waitErr, context.Canceled), errors.Is(waitErr, context.DeadlineExceeded):
		outcome = "aborted"
	default:
		outcome = "could not run"
	}

	attemptTimeout := "(none)"
	if p.AttemptTimeout > 0 {
		attemptTimeout = p.AttemptTimeout.String()
	}

	fmt.Fprintf(&b, "outcome:         %s\n", outcome)
	fmt.Fprintf(&b, "argv:            %v\n", p.Argv)
	fmt.Fprintf(&b, "timeout:         %s\n", p.Timeout)
	fmt.Fprintf(&b, "interval:        %s\n", p.Interval)
	fmt.Fprintf(&b, "attempt_timeout: %s\n", attemptTimeout)
	fmt.Fprintf(&b, "attempts:        %d\n", o.Attempts)
	fmt.Fprintf(&b, "elapsed:         %s\n", o.Elapsed.Round(time.Second))
	fmt.Fprintf(&b, "last attempt:    %s, exit code %d\n", o.LastKind, o.Last.ExitCode)
	fmt.Fprintf(&b, "last logs:       %s\n", o.LastLogs)
	if waitErr != nil {
		fmt.Fprintf(&b, "error:           %s\n", waitErr)
	}

	f := filepath.Join(dir, waitUntilSummaryName)
	if err := r.providerConf.Exec.WriteFile(ctx, f, []byte(b.String()), 0o640); err != nil {
		tflog.Debug(ctx, "wait_until: can't write the summary file", map[string]any{
			"file":  f,
			"error": err.Error(),
		})
	}
}

func (r *WaitUntil) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data WaitUntilModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	timeout, interval, attemptTimeout, dz := waitUntilDurations(&data)
	resp.Diagnostics.Append(dz...)
	if resp.Diagnostics.HasError() {
		return
	}

	interp, dz := r.resolveInterpreter(ctx, data.Interpreter)
	resp.Diagnostics.Append(dz...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, err := newResourceID()
	if err != nil {
		resp.Diagnostics.AddError("Wait Until Resource Error",
			fmt.Sprintf("Unable to generate a new resource ID: %s", err))
		return
	}
	data.ID = types.StringValue(id)
	ctx = tflog.SetField(ctx, "wait_until_id", id)

	d := r.getResourceDir(id)
	if err := r.providerConf.Exec.MkdirAll(ctx, d, 0o700); err != nil {
		resp.Diagnostics.AddError("Wait Until Resource Error",
			fmt.Sprintf("Unable to create resource specific directory: %s", err))
		return
	}
	if err := createTFBackPointer(ctx, r.providerConf.Exec, d); err != nil {
		resp.Diagnostics.AddError("Wait Until Resource Error",
			fmt.Sprintf("Unable to create resource specific file: %s", err))
		return
	}

	// The script is written once and re-executed BY PATH on every attempt. That
	// keeps the script text out of every attempt's *_command.log and out of
	// Result.Args (unlike `bash -c <script>`), and it lets a shebang work.
	scriptPath := filepath.Join(d, waitUntilScriptName)
	if err := r.providerConf.Exec.WriteFile(ctx, scriptPath, []byte(data.Script.ValueString()), 0o755); err != nil {
		resp.Diagnostics.AddError("Wait Until Resource Error",
			fmt.Sprintf("Unable to write the script on the target: %s", err))
		return
	}
	// os.WriteFile applies the umask when creating and does not chmod an existing
	// file, so make the executable bits explicit.
	if err := r.providerConf.Exec.Chmod(ctx, scriptPath, 0o755); err != nil {
		resp.Diagnostics.AddError("Wait Until Resource Error",
			fmt.Sprintf("Unable to make the script executable on the target: %s", err))
		return
	}
	data.ScriptPath = types.StringValue(scriptPath)

	p := waitParams{
		AttemptsRoot:   filepath.Join(d, waitUntilAttemptsDirName),
		Argv:           waitUntilArgv(interp, scriptPath),
		Timeout:        timeout,
		Interval:       interval,
		AttemptTimeout: attemptTimeout,
		RetainLogs:     waitUntilRetainLogs,
	}

	tflog.Info(ctx, "wait_until: waiting for the condition", map[string]any{
		"argv":     p.Argv,
		"timeout":  p.Timeout.String(),
		"interval": p.Interval.String(),
	})

	out, waitErr := waitForScript(ctx, r.providerConf.Exec, p)
	r.writeSummary(ctx, d, p, out, waitErr)

	if waitErr != nil {
		// The resource directory is deliberately NOT removed: it holds the script,
		// the summary and the retained attempt logs, which are the only record of
		// why the wait failed. No partial state is set either, so Terraform does
		// not end up with a tainted object.
		resp.Diagnostics.AddError(waitUntilErrDiag(out, p, d, waitErr))
		return
	}

	data.Attempts = types.Int64Value(int64(out.Attempts))
	data.Elapsed = types.StringValue(out.Elapsed.Round(time.Second).String())
	data.Stdout = types.StringValue(tailString(out.Last.Stdout, waitUntilOutputTailBytes))
	data.Stderr = types.StringValue(tailString(out.Last.Stderr, waitUntilOutputTailBytes))

	tflog.Trace(ctx, "WaitUntil Resource created successfully")

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Read is intentionally a no-op and must NEVER re-run the script. This resource
// records an event ("the probe exited 0 at time T"), not a live target-side
// artifact, so there is nothing to reconcile — and re-running it would mean an
// unrelated `plan` could block for the whole timeout. It also keeps refresh free
// of any SSH round-trip. Use `-replace=zedamigo_wait_until.NAME` to re-run the
// barrier on purpose.
func (r *WaitUntil) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data WaitUntilModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Update is only reachable for timeout/interval/attempt_timeout — script,
// interpreter and triggers all force replacement. The barrier already succeeded,
// so re-running it for a tuning change would be both slow and semantically
// wrong: carry the recorded outcome through unchanged and touch nothing on the
// target.
func (r *WaitUntil) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state WaitUntilModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	plan.ID = state.ID
	plan.ScriptPath = state.ScriptPath
	plan.Attempts = state.Attempts
	plan.Elapsed = state.Elapsed
	plan.Stdout = state.Stdout
	plan.Stderr = state.Stderr

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete only removes the bookkeeping directory: there is nothing on the target
// to undo, since by contract the script is a read-only probe. Exec.Remove has
// os.RemoveAll semantics and is idempotent for a missing path.
func (r *WaitUntil) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data WaitUntilModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d := r.getResourceDir(data.ID.ValueString())
	if err := r.providerConf.Exec.Remove(ctx, d); err != nil {
		resp.Diagnostics.AddError("Wait Until Resource Delete Error",
			fmt.Sprintf("Can't delete WaitUntil resource directory: %v", err))
		return
	}
}

// ImportState is a plain passthrough for consistency with the other resources,
// but note that `script` is required and forces replacement, so an import is
// always followed by a plan that replaces the resource: this is for state
// surgery only.
func (r *WaitUntil) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
