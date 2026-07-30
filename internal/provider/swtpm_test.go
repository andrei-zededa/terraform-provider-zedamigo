// SPDX-License-Identifier: MPL-2.0

//go:build linux

package provider

import (
	"context"
	"io"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/matryer/is"

	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/exec"
	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/hypervisor"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// swtpmHarness drives the SwTPM resource helpers through a real LocalExecutor
// and a real swtpm process, exactly the way the resource uses them in
// production: a fresh resource dir under a temp lib_path, the embedded
// process monitor script and the swtpm binary found on PATH.
type swtpmHarness struct {
	t   *testing.T
	ctx context.Context
	r   *SwTPM
	dir string
}

func newSwTPMHarness(t *testing.T) *swtpmHarness {
	t.Helper()
	is := is.New(t)
	ctx := context.Background()
	ex := exec.NewLocal(false)

	bash, err := ex.LookPath(ctx, "bash")
	if err != nil {
		t.Skip("bash not available on this host; skipping swtpm tests")
	}
	swtpm, err := ex.LookPath(ctx, "swtpm")
	if err != nil {
		t.Skip("swtpm not available on this host; skipping swtpm tests")
	}
	// Mirror the provider Configure step, which canonicalizes the swtpm path
	// so that readSwTPMPID's /proc/<pid>/exe comparison matches.
	if resolved, err := filepath.EvalSymlinks(swtpm); err == nil {
		swtpm = resolved
	}

	r := &SwTPM{providerConf: &ZedAmigoProviderConfig{
		LibPath: t.TempDir(),
		Bash:    bash,
		Swtpm:   swtpm,
		Exec:    ex,
	}}

	id, err := newResourceID()
	is.NoErr(err)
	dir := r.getResourceDir(id)
	is.NoErr(r.setupResourceDir(ctx, dir))

	h := &swtpmHarness{t: t, ctx: ctx, r: r, dir: dir}
	t.Cleanup(func() {
		// Never leak processes, even when an assertion fails mid-test.
		_ = h.r.stopSwTPM(ctx, dir)
	})
	return h
}

// start brings the monitor + swtpm stack up and waits for it to be ready.
func (h *swtpmHarness) start() {
	h.t.Helper()
	is := is.New(h.t)
	is.NoErr(h.r.startSwTPM(h.ctx, h.dir))
	is.NoErr(h.r.waitSwTPMReady(h.ctx, h.dir))
}

// pids returns the PIDs of the running monitor and swtpm processes, asserting
// that both are alive.
func (h *swtpmHarness) pids() (int, int) {
	h.t.Helper()
	is := is.New(h.t)
	pmRunning, pmPID, err := readMonitorPID(h.ctx, h.r.providerConf, h.dir)
	is.NoErr(err)
	is.True(pmRunning) // the process monitor must be running
	swRunning, swPID, err := readSwTPMPID(h.ctx, h.r.providerConf, h.dir)
	is.NoErr(err)
	is.True(swRunning) // the swtpm process must be running
	return pmPID, swPID
}

// gone asserts that pid dies within a short window. Process death is
// asynchronous after a signal, so a killed process can be visible as a
// corpse-in-exit for a few more milliseconds.
func (h *swtpmHarness) gone(pid int) {
	h.t.Helper()
	h.waitFor(2*time.Second, "process exits", func() bool {
		running, _ := h.r.providerConf.Exec.IsRunning(h.ctx, pid, "")
		return !running
	})
}

// noRespawn asserts that nothing restarts the stack after a stop: the monitor
// restart delay is 1s, so after a longer pause the PID files must not point
// to any newly started monitor or swtpm process.
func (h *swtpmHarness) noRespawn() {
	h.t.Helper()
	is := is.New(h.t)
	time.Sleep(2 * time.Second)
	pmRunning, _, _ := readMonitorPID(h.ctx, h.r.providerConf, h.dir)
	is.True(!pmRunning) // no monitor process must have been respawned
	swRunning, _, _ := readSwTPMPID(h.ctx, h.r.providerConf, h.dir)
	is.True(!swRunning) // no swtpm process must have been respawned
}

// sendCtrlShutdown does to the swtpm what QEMU does on every graceful VM
// shutdown: it sends CMD_SHUTDOWN (0x00000003) over the control channel,
// which swtpm always honors by exiting.
func (h *swtpmHarness) sendCtrlShutdown() {
	h.t.Helper()
	is := is.New(h.t)
	socketPath := filepath.Join(h.dir, swtpmSocketFile)
	conn, err := h.r.providerConf.Exec.Dial(h.ctx, "unix", socketPath, 5*time.Second)
	is.NoErr(err)
	_, err = conn.Write([]byte{0x00, 0x00, 0x00, 0x03}) // CMD_SHUTDOWN
	is.NoErr(err)
	// swtpm replies with a 4-byte success code before exiting; the read result
	// is irrelevant, it only sequences the caller after swtpm processed the
	// command.
	_, _ = io.ReadFull(conn, make([]byte, 4))
	conn.Close()
}

// waitFor polls cond until it is true, failing the test after timeout.
func (h *swtpmHarness) waitFor(timeout time.Duration, what string, cond func() bool) {
	h.t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	h.t.Fatalf("condition not met within %s: %s", timeout, what)
}

func TestSwTPMStartStop(t *testing.T) {
	is := is.New(t)
	h := newSwTPMHarness(t)

	h.start()
	pmPID, swPID := h.pids()

	_, err := h.r.providerConf.Exec.Stat(h.ctx, filepath.Join(h.dir, swtpmSocketFile))
	is.NoErr(err) // the control socket must exist

	// Stop must terminate BOTH the monitor and the swtpm process; the
	// historical bug was an orphaned swtpm surviving the resource delete.
	is.NoErr(h.r.stopSwTPM(h.ctx, h.dir))
	h.gone(pmPID)
	h.gone(swPID)

	// Stopping an already-stopped stack is a no-op.
	is.NoErr(h.r.stopSwTPM(h.ctx, h.dir))
}

// TestSwTPMRestartAfterVMShutdown simulates what QEMU does on every graceful
// VM shutdown: it sends CMD_SHUTDOWN (0x00000003) over the control channel,
// which swtpm always honors by exiting. The process monitor must restart
// swtpm on the same socket so that the next VM (e.g. the edge node booting
// after the EVE-OS installer VM) finds the same TPM again.
func TestSwTPMRestartAfterVMShutdown(t *testing.T) {
	is := is.New(t)
	h := newSwTPMHarness(t)

	h.start()
	_, oldPID := h.pids()

	socketPath := filepath.Join(h.dir, swtpmSocketFile)
	h.sendCtrlShutdown()

	h.waitFor(5*time.Second, "old swtpm process exits after CMD_SHUTDOWN", func() bool {
		running, _ := h.r.providerConf.Exec.IsRunning(h.ctx, oldPID, "")
		return !running
	})

	h.waitFor(10*time.Second, "monitor restarts swtpm", func() bool {
		running, pid, _ := readSwTPMPID(h.ctx, h.r.providerConf, h.dir)
		return running && pid != oldPID
	})
	is.NoErr(h.r.waitSwTPMReady(h.ctx, h.dir))

	// The restarted swtpm must accept control channel connections again.
	conn, err := h.r.providerConf.Exec.Dial(h.ctx, "unix", socketPath, 5*time.Second)
	is.NoErr(err)
	conn.Close()
}

// TestSwTPMVMStartRace reproduces the installer -> installed node race: the
// previous VM's graceful shutdown kills swtpm, and the next VM's QEMU is
// launched before the monitor's ~1s restart delay has elapsed. QEMU cannot
// retry the TPM chardev connection, so hypervisor.WaitForSwTPMSocket must
// bridge the restart window before QEMU is started.
func TestSwTPMVMStartRace(t *testing.T) {
	is := is.New(t)
	h := newSwTPMHarness(t)

	h.start()
	_, oldPID := h.pids()
	socketPath := filepath.Join(h.dir, swtpmSocketFile)

	h.sendCtrlShutdown()
	// Wait for the old swtpm to be fully gone, so the wait below runs against
	// the stale socket file inside the restart window, exactly like a QEMU
	// start racing the monitor.
	h.waitFor(5*time.Second, "old swtpm process exits after CMD_SHUTDOWN", func() bool {
		running, _ := h.r.providerConf.Exec.IsRunning(h.ctx, oldPID, "")
		return !running
	})

	// This is what QEMUHypervisor.Start now does before launching QEMU.
	is.NoErr(hypervisor.WaitForSwTPMSocket(h.ctx, h.r.providerConf.Exec, socketPath, 15*time.Second))

	// The socket accepted a connection, so a QEMU started now would find a
	// live swtpm: the restarted process, not the one that shut down.
	running, newPID, err := readSwTPMPID(h.ctx, h.r.providerConf, h.dir)
	is.NoErr(err)
	is.True(running)          // the restarted swtpm must be running
	is.True(newPID != oldPID) // and be a new process
}

// TestSwTPMStopEscalation covers the SIGKILL escalation path: a monitor that
// does not react to SIGTERM (simulated with SIGSTOP) must not be able to keep
// the stack alive or restart swtpm; stopSwTPM must kill the monitor first and
// then the swtpm process explicitly.
func TestSwTPMStopEscalation(t *testing.T) {
	is := is.New(t)
	h := newSwTPMHarness(t)

	h.start()
	pmPID, swPID := h.pids()

	is.NoErr(h.r.providerConf.Exec.Kill(h.ctx, pmPID, syscall.SIGSTOP))

	is.NoErr(h.r.stopSwTPM(h.ctx, h.dir))
	h.gone(pmPID)
	h.gone(swPID)
	h.noRespawn()
}

func TestSwTPMReadSelfHeal(t *testing.T) {
	is := is.New(t)
	h := newSwTPMHarness(t)

	// On a fresh (stopped) resource dir, readSwTPM must start the stack when
	// the desired state is "running"; this is also the Create code path.
	data := &SwTPMModel{State: types.StringValue("running")}
	is.NoErr(h.r.readSwTPM(h.ctx, h.dir, data))
	is.Equal(data.State.ValueString(), "running")
	is.Equal(data.Socket.ValueString(), filepath.Join(h.dir, swtpmSocketFile))
	pmPID, swPID := h.pids()

	// Kill the whole stack out-of-band (monitor first so that it cannot
	// restart swtpm), as after a host reboot; the next read must restart it.
	is.NoErr(h.r.providerConf.Exec.Kill(h.ctx, pmPID, syscall.SIGKILL))
	is.NoErr(h.r.providerConf.Exec.Kill(h.ctx, swPID, syscall.SIGKILL))
	h.waitFor(5*time.Second, "stack is down after SIGKILL", func() bool {
		pmRunning, _ := h.r.providerConf.Exec.IsRunning(h.ctx, pmPID, "")
		swRunning, _ := h.r.providerConf.Exec.IsRunning(h.ctx, swPID, "")
		return !pmRunning && !swRunning
	})

	is.NoErr(h.r.readSwTPM(h.ctx, h.dir, data))
	is.Equal(data.State.ValueString(), "running")
	newPMPID, newSWPID := h.pids()
	is.True(newPMPID != pmPID) // a new monitor must have been started
	is.True(newSWPID != swPID) // a new swtpm must have been started

	// And a desired state of "stopped" must stop the running stack.
	data.State = types.StringValue("stopped")
	is.NoErr(h.r.readSwTPM(h.ctx, h.dir, data))
	is.Equal(data.State.ValueString(), "stopped")
	h.gone(newPMPID)
	h.gone(newSWPID)
}

func TestSwTPMReadMissingDir(t *testing.T) {
	is := is.New(t)
	h := newSwTPMHarness(t)

	data := &SwTPMModel{State: types.StringValue("running")}
	err := h.r.readSwTPM(h.ctx, filepath.Join(h.dir, "nonexistent"), data)
	is.True(err != nil) // reading a missing resource dir must fail...
	// ... with the error Read uses to detect an externally deleted resource.
	is.True(err.Error() == "resource directory does not exist")
}
