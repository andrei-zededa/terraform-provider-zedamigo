// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"

	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/exec"
)

// sysInfo is the set of facts the zedamigo_system_info data source reports. It
// always describes the TARGET host, which is not necessarily the host the
// provider process runs on.
type sysInfo struct {
	Hostname       string
	CPUs           int
	MemTotalBytes  uint64
	MemUsedBytes   uint64
	MemUsedPercent float64
}

// gatherSysInfo collects the target's system info, reading it over the executor
// so that a remote target reports its own numbers rather than the provider
// host's.
func gatherSysInfo(ctx context.Context, ex exec.Executor, targetOS string) (sysInfo, error) {
	if ex == nil || ex.IsLocal() {
		return localSysInfo()
	}

	// Remote targets are Linux-only: the vfkit backend refuses a remote macOS
	// target, so /proc is always available here. Check anyway rather than let
	// this surface as a confusing "no such file: /proc/meminfo".
	if targetOS != "" && targetOS != "linux" {
		return sysInfo{}, fmt.Errorf("reading system info from a remote %s target is not supported (only linux is)", targetOS)
	}

	return remoteSysInfo(ctx, ex)
}

// localSysInfo reads the info for a local target via gopsutil, which is
// portable across linux and darwin.
func localSysInfo() (sysInfo, error) {
	var si sysInfo

	h, err := os.Hostname()
	if err != nil {
		return si, fmt.Errorf("can't get the system hostname: %w", err)
	}
	if h == "" {
		return si, fmt.Errorf("can't get the system hostname: got an empty string")
	}
	si.Hostname = h

	n, err := cpu.Counts(true)
	if err != nil {
		return si, fmt.Errorf("can't get the system CPU info: %w", err)
	}
	si.CPUs = n

	v, err := mem.VirtualMemory()
	if err != nil {
		return si, fmt.Errorf("can't get the system memory info: %w", err)
	}
	si.MemTotalBytes = v.Total
	si.MemUsedBytes = v.Used
	si.MemUsedPercent = v.UsedPercent

	return si, nil
}

// remoteSysInfo reads the info straight out of the target's /proc. Plain file
// reads over SFTP, so it needs no extra executables on the target.
func remoteSysInfo(ctx context.Context, ex exec.Executor) (sysInfo, error) {
	var si sysInfo

	h, err := ex.ReadFile(ctx, "/proc/sys/kernel/hostname")
	if err != nil {
		return si, fmt.Errorf("can't read the target hostname: %w", err)
	}
	si.Hostname = strings.TrimSpace(string(h))
	if si.Hostname == "" {
		return si, fmt.Errorf("can't read the target hostname: got an empty string")
	}

	ci, err := ex.ReadFile(ctx, "/proc/cpuinfo")
	if err != nil {
		return si, fmt.Errorf("can't read the target CPU info: %w", err)
	}
	si.CPUs, err = parseCPUCount(ci)
	if err != nil {
		return si, err
	}

	mi, err := ex.ReadFile(ctx, "/proc/meminfo")
	if err != nil {
		return si, fmt.Errorf("can't read the target memory info: %w", err)
	}
	si.MemTotalBytes, si.MemUsedBytes, si.MemUsedPercent, err = parseMemInfo(mi)
	if err != nil {
		return si, err
	}

	return si, nil
}

// parseCPUCount counts the logical CPUs in /proc/cpuinfo contents, matching
// what gopsutil's cpu.Counts(true) reports for a local target: one "processor"
// entry per logical CPU. Deliberately not `nproc`, whose default output is
// limited by the calling process' CPU affinity.
func parseCPUCount(cpuinfo []byte) (int, error) {
	n := 0
	for _, line := range strings.Split(string(cpuinfo), "\n") {
		key, _, ok := strings.Cut(line, ":")
		if ok && strings.TrimSpace(key) == "processor" {
			n++
		}
	}
	if n == 0 {
		return 0, fmt.Errorf("can't determine the target CPU count: no 'processor' entries in /proc/cpuinfo")
	}
	return n, nil
}

// parseMemInfo derives total/used memory from /proc/meminfo contents.
//
// "Used" follows procps `free`: total - free - buffers - (cached + reclaimable
// slab). That is also how gopsutil reports it on Linux, so a local and a remote
// Linux target stay comparable.
func parseMemInfo(meminfo []byte) (total, used uint64, usedPercent float64, err error) {
	fields := map[string]uint64{}

	for _, line := range strings.Split(string(meminfo), "\n") {
		key, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}

		// Values are "<number> kB", except a couple of unitless ones we ignore.
		f := strings.Fields(rest)
		if len(f) == 0 {
			continue
		}
		v, convErr := strconv.ParseUint(f[0], 10, 64)
		if convErr != nil {
			continue
		}
		if len(f) > 1 && f[1] == "kB" {
			v *= 1024
		}
		fields[strings.TrimSpace(key)] = v
	}

	total, ok := fields["MemTotal"]
	if !ok || total == 0 {
		return 0, 0, 0, fmt.Errorf("can't determine the target memory: no usable MemTotal in /proc/meminfo")
	}

	// Everything except MemTotal is best-effort: a missing field just means it
	// contributes nothing, which is better than failing the whole read.
	nonUsed := fields["MemFree"] + fields["Buffers"] + fields["Cached"] + fields["SReclaimable"]

	// Guard the subtraction: these are sampled non-atomically by the kernel, so
	// on a busy host the parts can briefly exceed the total.
	if nonUsed < total {
		used = total - nonUsed
	}

	usedPercent = float64(used) / float64(total) * 100

	return total, used, usedPercent, nil
}
