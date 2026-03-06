// SPDX-License-Identifier: MPL-2.0

//go:build !linux

package hypervisor

import "context"

// pinCPUThreads is a no-op on non-Linux platforms (CPU pinning not available).
func pinCPUThreads(_ context.Context, _ *QEMUHypervisor, _ int, _ []int64, _ int, _ string) error {
	return nil
}
