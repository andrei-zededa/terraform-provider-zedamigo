// SPDX-License-Identifier: MPL-2.0

//go:build linux

package provider

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/matryer/is"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/cmd/result"
	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/exec"
)

// waitHarness drives waitForScript through a real LocalExecutor against a script
// in a temp dir, exactly the way Create invokes it in production.
type waitHarness struct {
	ex         exec.Executor
	bash       string
	dir        string
	scriptPath string
	countFile  string
}

// waitCountToken is the placeholder a probe body uses to refer to the harness'
// count file. There is no env support in Executor.Run, so per-attempt state has
// to be carried in a file whose path is baked into the script.
const waitCountToken = "COUNT_FILE"

// newWaitHarness writes body as the probe script, substituting waitCountToken.
func newWaitHarness(t *testing.T, body string) *waitHarness {
	t.Helper()
	is := is.New(t)
	ctx := context.Background()
	ex := exec.NewLocal(false)

	bash, err := ex.LookPath(ctx, "bash")
	if err != nil {
		t.Skip("bash not available on this host; skipping wait_until tests")
	}

	dir := t.TempDir()
	h := &waitHarness{
		ex:         ex,
		bash:       bash,
		dir:        dir,
		scriptPath: filepath.Join(dir, waitUntilScriptName),
		countFile:  filepath.Join(dir, "count"),
	}

	is.NoErr(os.WriteFile(h.scriptPath, []byte(waitProbeBody(body, h.countFile)), 0o755))

	return h
}

// waitProbeBody substitutes the count file path into a probe body.
func waitProbeBody(body, countFile string) string {
	return strings.ReplaceAll(body, waitCountToken, countFile)
}

func (h *waitHarness) params(timeout, interval, attemptTimeout time.Duration, retain int) waitParams {
	return waitParams{
		AttemptsRoot:   filepath.Join(h.dir, waitUntilAttemptsDirName),
		Argv:           waitUntilArgv([]string{h.bash}, h.scriptPath),
		Timeout:        timeout,
		Interval:       interval,
		AttemptTimeout: attemptTimeout,
		RetainLogs:     retain,
	}
}

// attemptDirs lists the surviving per-attempt log directories.
func (h *waitHarness) attemptDirs(t *testing.T) []string {
	t.Helper()

	entries, err := os.ReadDir(filepath.Join(h.dir, waitUntilAttemptsDirName))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("can't read the attempts dir: %v", err)
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}

	return names
}

// countingProbe fails until it has been called n times, so a test can assert the
// exact number of attempts the loop made.
func countingProbe(n int) string {
	return fmt.Sprintf(`#!/usr/bin/env bash
i=0
[ -f "COUNT_FILE" ] && i=$(cat "COUNT_FILE")
i=$((i+1))
printf '%%s\n' "$i" > "COUNT_FILE"
echo "attempt $i"
if [ "$i" -lt %d ]; then
	echo "not ready at attempt $i" >&2
	exit 1
fi
`, n)
}

func TestWaitForScriptSucceedsAfterNAttempts(t *testing.T) {
	is := is.New(t)
	h := newWaitHarness(t, countingProbe(3))

	out, err := waitForScript(context.Background(), h.ex,
		h.params(10*time.Second, 20*time.Millisecond, 0, waitUntilRetainLogs))

	is.NoErr(err)
	is.Equal(out.Attempts, 3)
	is.Equal(out.LastKind, probeReady)
	is.Equal(out.Last.ExitCode, 0)
	is.True(strings.Contains(out.Last.Stdout, "attempt 3"))
	// The successful attempt's own logs are always kept.
	is.True(strings.Contains(out.LastLogs, "0003"))
}

func TestWaitForScriptTimesOut(t *testing.T) {
	is := is.New(t)
	h := newWaitHarness(t, `#!/usr/bin/env bash
echo "still waiting"
echo "nope" >&2
exit 7
`)

	out, err := waitForScript(context.Background(), h.ex,
		h.params(300*time.Millisecond, 20*time.Millisecond, 0, waitUntilRetainLogs))

	is.True(errors.Is(err, errWaitTimeout))
	is.True(out.Attempts >= 2)
	// The loop must never return before the whole budget is spent.
	is.True(out.Elapsed >= 300*time.Millisecond)
	// Assert on the last COMPLETED attempt, not on out.Last: the attempt in
	// flight when the deadline arrives gets only the sliver of budget that is
	// left, so whether it finished or was abandoned is inherently a race.
	is.True(out.LastCompletedAttempt >= 1)
	is.Equal(out.LastCompleted.ExitCode, 7)
	is.True(strings.Contains(out.LastCompleted.Stderr, "nope"))
}

// TestWaitForScriptAttemptTimeout exercises the per-attempt budget. NOTE: it
// intentionally leaves a few short-lived `sleep` processes behind — abandoning an
// attempt does not kill it, which is exactly the documented behavior.
func TestWaitForScriptAttemptTimeout(t *testing.T) {
	is := is.New(t)
	h := newWaitHarness(t, `#!/usr/bin/env bash
sleep 5
`)

	out, err := waitForScript(context.Background(), h.ex,
		h.params(400*time.Millisecond, 10*time.Millisecond, 100*time.Millisecond, waitUntilRetainLogs))

	is.True(errors.Is(err, errWaitTimeout))
	// Every attempt outlives its budget, so the last one must be abandoned.
	is.Equal(out.LastKind, probeHung)
	is.True(out.Last.TimedOut)
	is.True(out.Attempts >= 2)
	// An abandoned attempt's log dir is never pruned: it may still be written to.
	is.Equal(len(h.attemptDirs(t)), out.Attempts)
}

// TestWaitForScriptExecErrorFailsFast checks that a probe which cannot be started
// at all aborts on the first attempt instead of burning the whole timeout.
func TestWaitForScriptExecErrorFailsFast(t *testing.T) {
	is := is.New(t)
	h := newWaitHarness(t, `#!/usr/bin/env bash
exit 0
`)

	p := h.params(10*time.Second, 5*time.Second, 0, waitUntilRetainLogs)
	p.Argv = waitUntilArgv([]string{filepath.Join(h.dir, "no-such-interpreter")}, h.scriptPath)

	start := time.Now()
	out, err := waitForScript(context.Background(), h.ex, p)
	elapsed := time.Since(start)

	is.True(err != nil)
	is.True(!errors.Is(err, errWaitTimeout))
	is.Equal(out.Attempts, 1)
	is.Equal(out.LastKind, probeExecError)
	// It must not have waited out even a single interval.
	is.True(elapsed < time.Second)
	is.True(strings.Contains(err.Error(), "could not be run on the target"))
}

func TestWaitForScriptContextCancel(t *testing.T) {
	is := is.New(t)
	h := newWaitHarness(t, `#!/usr/bin/env bash
exit 1
`)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	defer cancel()

	// A long interval means the cancellation lands while the loop is sleeping.
	out, err := waitForScript(ctx, h.ex,
		h.params(30*time.Second, 5*time.Second, 0, waitUntilRetainLogs))

	is.True(errors.Is(err, context.Canceled))
	is.True(!errors.Is(err, errWaitTimeout))
	is.True(out.Attempts >= 1)
	is.True(out.Elapsed < 5*time.Second)
}

// TestWaitForScriptPrunesAttemptLogs checks the pruning call site: the attempts
// tree must stay bounded even though the default 30m/15s would otherwise leave
// ~120 dirs (3 log files each) behind.
func TestWaitForScriptPrunesAttemptLogs(t *testing.T) {
	is := is.New(t)
	h := newWaitHarness(t, countingProbe(10))

	keep := 3
	out, err := waitForScript(context.Background(), h.ex,
		h.params(30*time.Second, time.Millisecond, 0, keep))

	is.NoErr(err)
	is.Equal(out.Attempts, 10)

	dirs := h.attemptDirs(t)
	// The first attempt, the last `keep` COMPLETED ones (0006-0008 are pruned
	// down to the newest three, i.e. 0007-0009) and the successful one, which the
	// loop returns on before pruning.
	is.Equal(len(dirs), keep+2)
	is.True(contains(dirs, "0001"))
	is.True(contains(dirs, "0010"))
	is.True(!contains(dirs, "0002"))
}

func TestPruneAttemptLogs(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	ex := exec.NewLocal(false)

	root := t.TempDir()
	keep := 3

	// Mirror the loop's incremental call pattern: one dir per attempt, pruned
	// right after that attempt finished.
	var completed []string
	for i := 1; i <= 8; i++ {
		d := filepath.Join(root, fmt.Sprintf("%04d", i))
		mustMkdir(t, d)
		completed = pruneAttemptLogs(ctx, ex, append(completed, d), keep)
	}

	is.Equal(len(completed), keep+1)
	is.Equal(filepath.Base(completed[0]), "0001") // always spared

	entries, err := os.ReadDir(root)
	is.NoErr(err)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	is.Equal(names, []string{"0001", "0006", "0007", "0008"})
}

func TestClassifyProbe(t *testing.T) {
	is := is.New(t)
	boom := errors.New("boom")

	for _, tc := range []struct {
		name string
		res  result.Result
		want probeKind
	}{
		{"exit 0", result.Result{}, probeReady},
		{"exit 1", result.Result{ExitCode: 1, Error: boom}, probeNotReady},
		{"signal killed locally", result.Result{ExitCode: -1, Error: boom}, probeNotReady},
		{"could not start", result.Result{ExitCode: 0, Error: boom}, probeExecError},
	} {
		got := classifyProbe(tc.res)
		if got != tc.want {
			t.Errorf("%s: classifyProbe = %v, want %v", tc.name, got, tc.want)
		}
	}

	is.Equal(probeReady.String(), "ready")
	is.Equal(probeHung.String(), "abandoned")
}

func TestWaitUntilArgv(t *testing.T) {
	is := is.New(t)

	is.Equal(waitUntilArgv([]string{"/bin/bash"}, "/tmp/s.sh"), []string{"/bin/bash", "/tmp/s.sh"})
	is.Equal(waitUntilArgv([]string{"/bin/sh", "-e"}, "/tmp/s.sh"), []string{"/bin/sh", "-e", "/tmp/s.sh"})
	// An empty interpreter executes the script directly, via its shebang.
	is.Equal(waitUntilArgv(nil, "/tmp/s.sh"), []string{"/tmp/s.sh"})

	// A slice with spare capacity (as ElementsAs can return) must not be aliased.
	interp := make([]string, 1, 4)
	interp[0] = "/bin/bash"
	argv := waitUntilArgv(interp, "/tmp/s.sh")
	argv[0] = "/mutated"
	is.Equal(interp[0], "/bin/bash")
}

func TestTailString(t *testing.T) {
	is := is.New(t)

	is.Equal(tailString("short", 100), "short")
	is.Equal(tailString("", 100), "")
	is.Equal(tailString("abcdef", 0), "abcdef") // n <= 0 disables truncation
	is.Equal(tailString("abcdef", 3), "...[truncated]...\ndef")
}

func TestIndentLines(t *testing.T) {
	is := is.New(t)

	is.Equal(indentLines("a\nb"), "  a\n  b")
	is.Equal(indentLines("a\n"), "  a")
}

func TestWaitUntilDurations(t *testing.T) {
	is := is.New(t)

	// Defaults apply when the attributes are null, and attempt_timeout stays 0
	// ("bounded only by the overall timeout").
	timeout, interval, attemptTimeout, dz := waitUntilDurations(&WaitUntilModel{
		Timeout:        types.StringNull(),
		Interval:       types.StringNull(),
		AttemptTimeout: types.StringNull(),
	})
	is.True(!dz.HasError())
	is.Equal(timeout, 30*time.Minute)
	is.Equal(interval, 15*time.Second)
	is.Equal(attemptTimeout, time.Duration(0))

	timeout, interval, attemptTimeout, dz = waitUntilDurations(&WaitUntilModel{
		Timeout:        types.StringValue("1h30m"),
		Interval:       types.StringValue("250ms"),
		AttemptTimeout: types.StringValue("45s"),
	})
	is.True(!dz.HasError())
	is.Equal(timeout, 90*time.Minute)
	is.Equal(interval, 250*time.Millisecond)
	is.Equal(attemptTimeout, 45*time.Second)

	// A malformed value can only come from hand-edited state (the schema
	// validator rejects it at plan time), so it must be a hard error.
	_, _, _, dz = waitUntilDurations(&WaitUntilModel{
		Timeout:        types.StringValue("30"),
		Interval:       types.StringNull(),
		AttemptTimeout: types.StringNull(),
	})
	is.True(dz.HasError())
}

func TestWaitUntilErrDiag(t *testing.T) {
	is := is.New(t)

	p := waitParams{Timeout: 30 * time.Minute, Interval: 15 * time.Second}
	o := waitOutcome{
		Attempts: 120,
		Elapsed:  30 * time.Minute,
		LastKind: probeNotReady,
		Last:     result.Result{ExitCode: 1, Stderr: "port 50277 NOT ready yet."},
	}

	summary, detail := waitUntilErrDiag(o, p, "/var/lib/zedamigo/wait_until/abcd1234", errWaitTimeout)
	is.Equal(summary, "Timed out waiting for the condition")
	is.True(strings.Contains(detail, "did not exit 0 within 30m0s"))
	is.True(strings.Contains(detail, "attempts: 120"))
	is.True(strings.Contains(detail, "last attempt exit code: 1"))
	is.True(strings.Contains(detail, "  port 50277 NOT ready yet."))
	is.True(strings.Contains(detail, "/var/lib/zedamigo/wait_until/abcd1234"))

	// When the final attempt was abandoned it carries no output of its own, so the
	// diagnostic must fall back to the last attempt that actually finished —
	// otherwise a timeout would report no reason at all.
	o.LastKind = probeHung
	o.Last = result.Result{TimedOut: true}
	o.LastCompleted = result.Result{ExitCode: 1, Stderr: "port 50277 NOT ready yet."}
	o.LastCompletedAttempt = 119
	_, detail = waitUntilErrDiag(o, p, "/tmp/x", errWaitTimeout)
	is.True(strings.Contains(detail, "abandoned"))
	is.True(strings.Contains(detail, "may still be running on the target"))
	is.True(strings.Contains(detail, "last completed attempt (#119) exit code: 1"))
	is.True(strings.Contains(detail, "  port 50277 NOT ready yet."))

	// With no completed attempt at all there is simply nothing to fall back to.
	o.LastCompleted, o.LastCompletedAttempt = result.Result{}, 0
	_, detail = waitUntilErrDiag(o, p, "/tmp/x", errWaitTimeout)
	is.True(strings.Contains(detail, "abandoned"))
	is.True(!strings.Contains(detail, "last completed attempt"))

	// A cancellation also leaves LastKind == probeHung, but must not blame the
	// per-attempt budget for it.
	o.LastKind = probeHung
	summary, detail = waitUntilErrDiag(o, p, "/tmp/x", context.Canceled)
	is.Equal(summary, "Waiting for the condition was aborted")
	is.True(strings.Contains(detail, "abandoned mid-run by the cancellation"))
	is.True(!strings.Contains(detail, "within its budget"))

	summary, _ = waitUntilErrDiag(o, p, "/tmp/x", errors.New("dial tcp: connection refused"))
	is.Equal(summary, "The script could not be run on the target")
}

func TestPositiveDurationValidator(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()

	for _, tc := range []struct {
		in      string
		wantErr bool
	}{
		{"30m", false},
		{"1h30m", false},
		{"250ms", false},
		{"", true},     // not a duration
		{"30", true},   // a bare number has no unit
		{"abc", true},  // not a duration
		{"0s", true},   // must be positive
		{"-1s", true},  // must be positive
		{"5min", true}, // "min" is not a Go duration unit
	} {
		req := validator.StringRequest{
			Path:        path.Root("timeout"),
			ConfigValue: types.StringValue(tc.in),
		}
		resp := &validator.StringResponse{}
		positiveDurationValidator{}.ValidateString(ctx, req, resp)

		if got := resp.Diagnostics.HasError(); got != tc.wantErr {
			t.Errorf("ValidateString(%q): HasError = %v, want %v (%v)",
				tc.in, got, tc.wantErr, resp.Diagnostics)
		}
	}

	// An unset or not-yet-known value is not this validator's business.
	for _, v := range []types.String{types.StringNull(), types.StringUnknown()} {
		resp := &validator.StringResponse{}
		positiveDurationValidator{}.ValidateString(ctx,
			validator.StringRequest{Path: path.Root("timeout"), ConfigValue: v}, resp)
		is.True(!resp.Diagnostics.HasError())
	}
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}

	return false
}
