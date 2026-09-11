// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"strings"
	"testing"
)

func TestParseCPUCount(t *testing.T) {
	// Two logical CPUs, with the trailing blank line /proc/cpuinfo really has.
	const twoCPUs = `processor	: 0
vendor_id	: GenuineIntel
cpu family	: 6
model name	: Intel(R) Core(TM) i7-1165G7 @ 2.80GHz
cpu MHz		: 2803.200

processor	: 1
vendor_id	: GenuineIntel
cpu family	: 6
model name	: Intel(R) Core(TM) i7-1165G7 @ 2.80GHz
cpu MHz		: 2803.200

`

	tests := []struct {
		name    string
		in      string
		want    int
		wantErr bool
	}{
		{name: "two logical cpus", in: twoCPUs, want: 2},
		{name: "single cpu", in: "processor\t: 0\nmodel name\t: whatever\n", want: 1},
		{
			// "processor" must be matched as a key, not as a substring: these
			// lines all contain the word but describe something else.
			name: "no false positives from other keys",
			in: "processor\t: 0\n" +
				"address sizes\t: 39 bits physical, 48 bits virtual\n" +
				"model name\t: Some processor with processor in the name\n",
			want: 1,
		},
		{name: "empty", in: "", wantErr: true},
		{name: "no processor entries", in: "MemTotal:  1024 kB\n", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseCPUCount([]byte(tt.in))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseCPUCount() = %d, want an error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseCPUCount() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("parseCPUCount() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestParseMemInfo(t *testing.T) {
	const k = 1024

	// Trimmed from a real Ubuntu 22.04 /proc/meminfo.
	meminfo := strings.Join([]string{
		"MemTotal:       32570036 kB",
		"MemFree:        23400000 kB",
		"MemAvailable:   29000000 kB",
		"Buffers:          200000 kB",
		"Cached:          6000000 kB",
		"SwapCached:            0 kB",
		"SReclaimable:     400000 kB",
		"HugePages_Total:       0", // unitless, must not be scaled by 1024
		"",
	}, "\n")

	total, used, pct, err := parseMemInfo([]byte(meminfo))
	if err != nil {
		t.Fatalf("parseMemInfo() unexpected error: %v", err)
	}

	if want := uint64(32570036) * k; total != want {
		t.Errorf("total = %d, want %d", total, want)
	}
	// free(1) semantics: total - free - buffers - (cached + reclaimable).
	if want := uint64(32570036-23400000-200000-6000000-400000) * k; used != want {
		t.Errorf("used = %d, want %d", used, want)
	}
	if pct <= 0 || pct >= 100 {
		t.Errorf("usedPercent = %v, want a value in (0, 100)", pct)
	}
}

func TestParseMemInfoUnitless(t *testing.T) {
	// A unitless MemTotal (no "kB" suffix) must be taken at face value rather
	// than silently multiplied by 1024.
	total, _, _, err := parseMemInfo([]byte("MemTotal: 4096\nMemFree: 1024\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 4096 {
		t.Errorf("total = %d, want 4096", total)
	}
}

func TestParseMemInfoErrors(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{name: "empty", in: ""},
		{name: "no MemTotal", in: "MemFree: 1024 kB\nCached: 512 kB\n"},
		{name: "zero MemTotal", in: "MemTotal: 0 kB\n"},
		{name: "unparseable MemTotal", in: "MemTotal: not-a-number kB\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, _, err := parseMemInfo([]byte(tt.in)); err == nil {
				t.Error("parseMemInfo() = nil error, want an error")
			}
		})
	}
}

func TestParseMemInfoDoesNotUnderflow(t *testing.T) {
	// The kernel samples these counters non-atomically, so on a busy host the
	// parts can briefly add up to more than MemTotal. Used must clamp to 0
	// rather than wrapping around a uint64.
	_, used, pct, err := parseMemInfo([]byte(
		"MemTotal: 1000 kB\nMemFree: 900 kB\nBuffers: 200 kB\nCached: 300 kB\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if used != 0 {
		t.Errorf("used = %d, want 0 (must not underflow)", used)
	}
	if pct != 0 {
		t.Errorf("usedPercent = %v, want 0", pct)
	}
}

// A local target must keep reporting via gopsutil, unchanged.
func TestGatherSysInfoLocal(t *testing.T) {
	si, err := gatherSysInfo(t.Context(), nil, "")
	if err != nil {
		t.Fatalf("gatherSysInfo() unexpected error: %v", err)
	}
	if si.Hostname == "" {
		t.Error("Hostname is empty")
	}
	if si.CPUs < 1 {
		t.Errorf("CPUs = %d, want >= 1", si.CPUs)
	}
	if si.MemTotalBytes == 0 {
		t.Error("MemTotalBytes is 0")
	}
}
