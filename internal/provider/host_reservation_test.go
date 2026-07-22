// SPDX-License-Identifier: MPL-2.0

//go:build linux

package provider

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/matryer/is"

	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/exec"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Fixed controller-side identity fields the harness passes to every reserve, so
// tests can assert on the stored marker without depending on the host's actual
// user/hostname (which the script fills into the remote-side fields).
const (
	testLocalUser = "tester"
	testLocalHost = "testhost"
	testConfigDir = "/tmp/cfg dir" // embedded space: must survive as one field
)

// scriptHarness drives the embedded host_reservation.bash through a real
// LocalExecutor against a fake capacity tree, exactly the way the resource
// invokes it in production.
type scriptHarness struct {
	root string
	run  func(mode, id string, extra ...string) (string, int)
}

func newScriptHarness(t *testing.T, ncpus, ngb int, devs []string) *scriptHarness {
	t.Helper()
	is := is.New(t)
	ctx := context.Background()
	ex := exec.NewLocal(false)

	bash, err := ex.LookPath(ctx, "bash")
	is.NoErr(err)
	flock, err := ex.LookPath(ctx, "flock")
	if err != nil {
		t.Skip("flock not available on this host; skipping host_reservation script tests")
	}

	logDir := t.TempDir()
	root := t.TempDir()
	// Only declare a category's directory when it has capacity — mirrors an
	// operator that offers, say, CPUs but not RAM.
	if ncpus > 0 {
		mustMkdir(t, filepath.Join(root, "cpus", "unit"))
		for i := 0; i < ncpus; i++ {
			mustTouch(t, filepath.Join(root, "cpus", "unit", strconv.Itoa(i)))
		}
	}
	if ngb > 0 {
		mustMkdir(t, filepath.Join(root, "ram", "gb"))
		for i := 0; i < ngb; i++ {
			mustTouch(t, filepath.Join(root, "ram", "gb", strconv.Itoa(i)))
		}
	}
	for _, d := range devs {
		mustTouch(t, filepath.Join(root, "devs", d))
	}

	run := func(mode, id string, extra ...string) (string, int) {
		full := []string{"-c", hostReservationScript, hostReservationArg0, mode, flock, id, root}
		if mode == "reserve" {
			// The resource always supplies the controller-side identity trio
			// (local user, local host, config dir) between "<ncpu> <nmem>" and the
			// variadic devs. Callers here pass just "<ncpu> <nmem> [dev...]", so
			// splice fixed test values in to mirror production invocation.
			full = append(full, extra[:2]...)
			full = append(full, testLocalUser, testLocalHost, testConfigDir)
			full = append(full, extra[2:]...)
		} else {
			full = append(full, extra...)
		}
		res, _ := ex.Run(ctx, logDir, bash, full...)
		return res.Stdout, res.ExitCode
	}
	return &scriptHarness{root: root, run: run}
}

func mustMkdir(t *testing.T, p string) {
	t.Helper()
	is := is.New(t)
	is.NoErr(os.MkdirAll(p, 0o755))
}

func mustTouch(t *testing.T, p string) {
	t.Helper()
	is := is.New(t)
	is.NoErr(os.MkdirAll(filepath.Dir(p), 0o755))
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY, 0o644)
	is.NoErr(err)
	is.NoErr(f.Close())
}

func fileSize(t *testing.T, p string) int64 {
	t.Helper()
	is := is.New(t)
	info, err := os.Stat(p)
	is.NoErr(err)
	return info.Size()
}

func TestHostReservationScriptLifecycle(t *testing.T) {
	is := is.New(t)
	h := newScriptHarness(t, 8, 16, []string{"/dev/sdb", "/dev/disk/by-id/foo"})

	// Reserve A: 4 CPUs, 4 GB, /dev/sdb -> lowest-numbered free slots.
	out, code := h.run("reserve", "aaaa1111", "4", "4", "/dev/sdb")
	is.Equal(code, 0)
	rr, err := parseReservedJSON(out)
	is.NoErr(err)
	is.Equal(rr.CPUs, []int64{0, 1, 2, 3})
	is.Equal(rr.Mem, []int64{0, 1, 2, 3})
	is.Equal(rr.Devs, []string{"/dev/sdb"})

	// Reserve B: 4 more CPUs -> next free slots, no overlap.
	out, code = h.run("reserve", "bbbb2222", "4", "0")
	is.Equal(code, 0)
	rr, err = parseReservedJSON(out)
	is.NoErr(err)
	is.Equal(rr.CPUs, []int64{4, 5, 6, 7})

	// Reserve C: 1 more CPU -> none free -> exit 4 (insufficient CPUs).
	_, code = h.run("reserve", "cccc3333", "1", "0")
	is.Equal(code, 4)

	// Reserve D: /dev/sdb already reserved -> exit 8.
	_, code = h.run("reserve", "dddd4444", "0", "0", "/dev/sdb")
	is.Equal(code, 8)

	// Reserve E: device with no capacity file -> exit 7.
	_, code = h.run("reserve", "eeee5555", "0", "0", "/dev/nope")
	is.Equal(code, 7)

	// scan A reproduces exactly what reserve A returned.
	out, code = h.run("scan", "aaaa1111")
	is.Equal(code, 0)
	rr, err = parseReservedJSON(out)
	is.NoErr(err)
	is.Equal(rr.CPUs, []int64{0, 1, 2, 3})
	is.Equal(rr.Mem, []int64{0, 1, 2, 3})
	is.Equal(rr.Devs, []string{"/dev/sdb"})

	// Release A frees its slots; scan then owns nothing.
	_, code = h.run("release", "aaaa1111")
	is.Equal(code, 0)
	out, code = h.run("scan", "aaaa1111")
	is.Equal(code, 0)
	rr, err = parseReservedJSON(out)
	is.NoErr(err)
	is.Equal(len(rr.CPUs), 0)
	is.Equal(len(rr.Mem), 0)
	is.Equal(len(rr.Devs), 0)

	// Release A again is idempotent.
	_, code = h.run("release", "aaaa1111")
	is.Equal(code, 0)

	// Reserve G reclaims the freed lowest slots.
	out, code = h.run("reserve", "ffff6666", "4", "0")
	is.Equal(code, 0)
	rr, err = parseReservedJSON(out)
	is.NoErr(err)
	is.Equal(rr.CPUs, []int64{0, 1, 2, 3})
}

// TestHostReservationScriptMarker verifies a claimed slot stores the full
// tab-separated ownership marker (id + local/remote identity + config dir) and
// that ownership-keyed operations (scan/release) still key on field 1 alone.
func TestHostReservationScriptMarker(t *testing.T) {
	is := is.New(t)
	h := newScriptHarness(t, 2, 0, nil)

	out, code := h.run("reserve", "abcd1234", "1", "0")
	is.Equal(code, 0)
	rr, err := parseReservedJSON(out)
	is.NoErr(err)
	is.Equal(len(rr.CPUs), 1)

	slot := filepath.Join(h.root, "cpus", "unit", strconv.FormatInt(rr.CPUs[0], 10))
	raw, err := os.ReadFile(slot)
	is.NoErr(err)

	// The record is a single line (no trailing newline) of six tab-separated
	// fields in the documented order.
	is.True(!strings.Contains(string(raw), "\n"))
	fields := strings.Split(string(raw), "\t")
	is.Equal(len(fields), 6)
	is.Equal(fields[0], "abcd1234")    // reservation id
	is.Equal(fields[1], testLocalUser) // local username (supplied by caller)
	is.True(fields[2] != "")           // remote username (resolved on target)
	is.Equal(fields[3], testLocalHost) // local hostname (supplied by caller)
	is.True(fields[4] != "")           // remote hostname (resolved on target)
	is.Equal(fields[5], testConfigDir) // config dir, embedded space preserved

	// Ownership keys on field 1: scan finds it, release clears it.
	out, code = h.run("scan", "abcd1234")
	is.Equal(code, 0)
	rr, err = parseReservedJSON(out)
	is.NoErr(err)
	is.Equal(len(rr.CPUs), 1)

	_, code = h.run("release", "abcd1234")
	is.Equal(code, 0)
	is.Equal(fileSize(t, slot), int64(0))
}

// TestHostReservationScriptLegacyMarker verifies backward compatibility: a slot
// written by an older provider (bare "<id>", no tabs) is still recognized as
// owned, so scan/release keep working across the upgrade.
func TestHostReservationScriptLegacyMarker(t *testing.T) {
	is := is.New(t)
	h := newScriptHarness(t, 2, 0, nil)

	// Simulate a legacy claim: the marker is just the id, with no tab fields.
	legacy := filepath.Join(h.root, "cpus", "unit", "0")
	is.NoErr(os.WriteFile(legacy, []byte("legacy01"), 0o644))

	out, code := h.run("scan", "legacy01")
	is.Equal(code, 0)
	rr, err := parseReservedJSON(out)
	is.NoErr(err)
	is.Equal(rr.CPUs, []int64{0})

	_, code = h.run("release", "legacy01")
	is.Equal(code, 0)
	is.Equal(fileSize(t, legacy), int64(0))
}

// TestHostReservationScriptRollback verifies the all-or-nothing property: a
// device failure must not leave any CPU/RAM marker written, because selection
// is fully validated before the commit phase writes anything.
func TestHostReservationScriptRollback(t *testing.T) {
	is := is.New(t)
	h := newScriptHarness(t, 4, 4, []string{"/dev/sdb"})

	_, code := h.run("reserve", "aaaa1111", "2", "2", "/dev/nope")
	is.Equal(code, 7)

	for i := 0; i < 4; i++ {
		is.Equal(fileSize(t, filepath.Join(h.root, "cpus", "unit", strconv.Itoa(i))), int64(0))
		is.Equal(fileSize(t, filepath.Join(h.root, "ram", "gb", strconv.Itoa(i))), int64(0))
	}

	out, code := h.run("scan", "aaaa1111")
	is.Equal(code, 0)
	rr, err := parseReservedJSON(out)
	is.NoErr(err)
	is.Equal(len(rr.CPUs), 0)
	is.Equal(len(rr.Mem), 0)
}

func TestHostReservationScriptMissingCapacity(t *testing.T) {
	is := is.New(t)
	// A tree with no cpus/ capacity dir at all.
	h := newScriptHarness(t, 0, 0, nil)

	_, code := h.run("reserve", "aaaa1111", "1", "0")
	is.Equal(code, 3) // no capacity dir declared
}

func TestParseReservedJSON(t *testing.T) {
	is := is.New(t)

	rr, err := parseReservedJSON(`{"cpus":[1,2,6,7],"mem":[0],"devs":["/dev/sdb"]}`)
	is.NoErr(err)
	is.Equal(rr.CPUs, []int64{1, 2, 6, 7})
	is.Equal(rr.Mem, []int64{0})
	is.Equal(rr.Devs, []string{"/dev/sdb"})

	// Surrounding whitespace/newlines are tolerated.
	rr, err = parseReservedJSON("  {\"cpus\":[],\"mem\":[],\"devs\":[]}\n")
	is.NoErr(err)
	is.Equal(len(rr.CPUs), 0)
	is.Equal(len(rr.Mem), 0)
	is.Equal(len(rr.Devs), 0)

	_, err = parseReservedJSON("   ")
	is.True(err != nil)

	_, err = parseReservedJSON("not json")
	is.True(err != nil)
}

func TestCleanValidateDevs(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()

	// Null list -> no devices, no error.
	out, diags := cleanValidateDevs(ctx, types.ListNull(types.StringType))
	is.Equal(len(out), 0)
	is.True(!diags.HasError())

	// Valid entries pass through in order.
	valid, _ := buildStringList([]string{"/dev/sdb", "/dev/disk/by-id/foo"})
	out, diags = cleanValidateDevs(ctx, valid)
	is.True(!diags.HasError())
	is.Equal(out, []string{"/dev/sdb", "/dev/disk/by-id/foo"})

	for _, bad := range [][]string{
		{"/dev/sdb", "/dev/sdb"}, // duplicate
		{"relative/path"},        // not absolute
		{"/etc/passwd"},          // not under /dev
		{"/dev/../etc/shadow"},   // contains ..
		{"/dev/foo bar"},         // whitespace
		{"/dev/"},                // too short
		{"/dev/a/"},              // unclean (trailing slash)
		{""},                     // empty
	} {
		l, _ := buildStringList(bad)
		_, diags := cleanValidateDevs(ctx, l)
		is.True(diags.HasError())
	}
}

func TestInt64OrZero(t *testing.T) {
	is := is.New(t)
	is.Equal(int64OrZero(types.Int64Null()), int64(0))
	is.Equal(int64OrZero(types.Int64Unknown()), int64(0))
	is.Equal(int64OrZero(types.Int64Value(7)), int64(7))
}
