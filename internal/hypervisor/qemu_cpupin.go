// SPDX-License-Identifier: MPL-2.0

package hypervisor

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/exec"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// getCPUThreadIDs reads the target's /proc filesystem to find QEMU vCPU thread
// IDs. Returns a map[cpuNum]threadID for threads named "CPU N/KVM".
func getCPUThreadIDs(ctx context.Context, ex exec.Executor, qemuPID int, numCPUs int) (map[int]int, error) {
	return getCPUThreadIDsFromDir(ctx, ex, fmt.Sprintf("/proc/%d/task", qemuPID), numCPUs)
}

// getCPUThreadIDsFromDir scans a task directory for QEMU vCPU threads.
// Extracted for testability — accepts an arbitrary directory path.
func getCPUThreadIDsFromDir(ctx context.Context, ex exec.Executor, taskDir string, numCPUs int) (map[int]int, error) {
	entries, err := ex.ReadDir(ctx, taskDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read task directory: %w", err)
	}

	cpuThreads := make(map[int]int)

	for _, entry := range entries {
		tid := entry.Name()
		commPath := filepath.Join(taskDir, tid, "comm")

		commBytes, err := ex.ReadFile(ctx, commPath)
		if err != nil {
			continue // Thread may have exited
		}

		comm := strings.TrimSpace(string(commBytes))

		// Match "CPU N/KVM" pattern
		if strings.HasPrefix(comm, "CPU ") && strings.HasSuffix(comm, "/KVM") {
			cpuNumStr := strings.TrimPrefix(comm, "CPU ")
			cpuNumStr = strings.TrimSuffix(cpuNumStr, "/KVM")

			cpuNum, err := strconv.Atoi(cpuNumStr)
			if err != nil {
				continue
			}

			tidInt, err := strconv.Atoi(tid)
			if err != nil {
				continue
			}

			cpuThreads[cpuNum] = tidInt
		}
	}

	if len(cpuThreads) != numCPUs {
		return cpuThreads, fmt.Errorf("expected %d CPU threads, found %d", numCPUs, len(cpuThreads))
	}

	return cpuThreads, nil
}

// pinCPUThreads pins QEMU vCPU threads to host CPU cores using taskset.
func pinCPUThreads(ctx context.Context, h *QEMUHypervisor, qemuPID int, cpuPins []int64, numCPUs int, logPath string) error {
	// Wait for QEMU threads to initialize
	time.Sleep(500 * time.Millisecond)

	// Retry logic: threads may take time to appear
	var cpuThreads map[int]int
	var err error
	for i := 0; i < 10; i++ {
		cpuThreads, err = getCPUThreadIDs(ctx, h.Exec, qemuPID, numCPUs)
		if err == nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	if err != nil {
		return fmt.Errorf("failed to find CPU threads after retries: %w", err)
	}

	// Pin each vCPU thread to corresponding host CPU.
	// Try all pins so the user sees ALL failures, not just the first.
	var pinErrors []string
	for cpuNum := 0; cpuNum < numCPUs; cpuNum++ {
		threadID, ok := cpuThreads[cpuNum]
		if !ok {
			pinErrors = append(pinErrors, fmt.Sprintf("CPU %d: thread not found", cpuNum))
			continue
		}

		hostCPU := cpuPins[cpuNum]

		tasksetArgs := []string{
			"-cp",
			fmt.Sprintf("%d", hostCPU),
			fmt.Sprintf("%d", threadID),
		}

		if h.UseSudo {
			args := append([]string{h.TasksetPath}, tasksetArgs...)
			_, err = h.Exec.Run(ctx, logPath, h.SudoPath, args...)
		} else {
			_, err = h.Exec.Run(ctx, logPath, h.TasksetPath, tasksetArgs...)
		}

		if err != nil {
			tflog.Warn(ctx, "Failed to pin CPU thread", map[string]any{
				"cpu_num":   cpuNum,
				"thread_id": threadID,
				"host_cpu":  hostCPU,
				"error":     err,
			})
			pinErrors = append(pinErrors, fmt.Sprintf("CPU %d (tid %d -> core %d): %v", cpuNum, threadID, hostCPU, err))
		} else {
			tflog.Debug(ctx, "Pinned CPU thread", map[string]any{
				"cpu_num":   cpuNum,
				"thread_id": threadID,
				"host_cpu":  hostCPU,
			})
		}
	}

	if len(pinErrors) > 0 {
		return fmt.Errorf("taskset failed for %d vCPU(s):\n  %s", len(pinErrors), strings.Join(pinErrors, "\n  "))
	}

	return nil
}
