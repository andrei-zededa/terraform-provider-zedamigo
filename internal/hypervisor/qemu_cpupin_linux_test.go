// SPDX-License-Identifier: MPL-2.0

//go:build linux

package hypervisor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/matryer/is"
)

func TestGetCPUThreadIDsFromDir(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		is := is.New(t)

		dir := t.TempDir()

		// Simulate /proc/<pid>/task with 2 vCPU threads + 1 main thread.
		for _, tc := range []struct {
			tid  string
			comm string
		}{
			{"100", "qemu-system-x86\n"},
			{"101", "CPU 0/KVM\n"},
			{"102", "CPU 1/KVM\n"},
		} {
			taskPath := filepath.Join(dir, tc.tid)
			is.NoErr(os.Mkdir(taskPath, 0o755))
			is.NoErr(os.WriteFile(filepath.Join(taskPath, "comm"), []byte(tc.comm), 0o644))
		}

		threads, err := getCPUThreadIDsFromDir(dir, 2)
		is.NoErr(err)
		is.Equal(len(threads), 2)
		is.Equal(threads[0], 101)
		is.Equal(threads[1], 102)
	})

	t.Run("wrong thread count", func(t *testing.T) {
		is := is.New(t)

		dir := t.TempDir()

		// Only 1 CPU thread but we expect 2.
		taskPath := filepath.Join(dir, "200")
		is.NoErr(os.Mkdir(taskPath, 0o755))
		is.NoErr(os.WriteFile(filepath.Join(taskPath, "comm"), []byte("CPU 0/KVM\n"), 0o644))

		_, err := getCPUThreadIDsFromDir(dir, 2)
		is.True(err != nil)
	})

	t.Run("missing directory", func(t *testing.T) {
		is := is.New(t)

		_, err := getCPUThreadIDsFromDir("/nonexistent/path", 2)
		is.True(err != nil)
	})
}
