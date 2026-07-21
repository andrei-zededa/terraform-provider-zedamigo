// SPDX-License-Identifier: MPL-2.0

//go:build linux

package provider

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/matryer/is"

	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/exec"
	"github.com/hashicorp/terraform-plugin-framework/types"
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
		full = append(full, extra...)
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
