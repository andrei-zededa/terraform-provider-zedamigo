package cmd

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/matryer/is"
)

// Test_ProcessDataIntoLines tests the processDataIntoLines function.
func Test_ProcessDataIntoLines(t *testing.T) {
	testCases := []struct {
		name string
		in   []byte
		want []string
	}{
		{
			name: "Empty input",
			in:   []byte{},
			want: []string{},
		},
		{
			name: "Single line without newline",
			in:   []byte("test line"),
			want: []string{"test line"},
		},
		{
			name: "Single line with LF",
			in:   []byte("test line\n"),
			want: []string{"test line"},
		},
		{
			name: "Single line with CR",
			in:   []byte("test line\r"),
			want: []string{"test line"},
		},
		{
			name: "Single line with CRLF",
			in:   []byte("test line\r\n"),
			want: []string{"test line"},
		},
		{
			name: "Multiple lines with LF",
			in:   []byte("line1\nline2\nline3\n"),
			want: []string{"line1", "line2", "line3"},
		},
		{
			name: "Multiple lines with CR",
			in:   []byte("line1\rline2\rline3\r"),
			want: []string{"line1", "line2", "line3"},
		},
		{
			name: "Multiple lines with CRLF",
			in:   []byte("line1\r\nline2\r\nline3\r\n"),
			want: []string{"line1", "line2", "line3"},
		},
		{
			name: "Mixed line endings",
			in:   []byte("line1\nline2\rline3\r\n"),
			want: []string{"line1", "line2", "line3"},
		},
		{
			name: "With partial line at end",
			in:   []byte("line1\nline2\npartial"),
			want: []string{"line1", "line2", "partial"},
		},
		{
			name: "Empty lines",
			in:   []byte("\n\n\n"),
			want: []string{},
		},
		{
			name: "Lines with empty content",
			in:   []byte("line1\n\nline3\n"),
			want: []string{"line1", "line3"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			is := is.New(t)
			got := intoLines(tc.in)
			is.Equal(got, tc.want)
			is.Equal(got, tc.want)
		})
	}
}

// Test_MonitorFileFor tests the monitorFileFor function.
func Test_MonitorFileFor(t *testing.T) {
	is := is.New(t)
	tmpDir, err := os.MkdirTemp("", "file-monitor-test")
	is.NoErr(err)
	defer os.RemoveAll(tmpDir)

	testCases := []struct {
		name          string
		fileContents  []string
		writeInterval time.Duration
		pattern       string
		shouldFind    bool
		timeout       time.Duration
	}{
		{
			name:          "Found immediately one line",
			fileContents:  []string{"text1TEST1001text3\n"},
			writeInterval: 0,
			pattern:       "TEST1001",
			shouldFind:    true,
			timeout:       1 * time.Second,
		},
		{
			name:          "Found immediately multiple line",
			fileContents:  []string{"line1\nTEST1001\nline3\n"},
			writeInterval: 0,
			pattern:       "TEST1001",
			shouldFind:    true,
			timeout:       1 * time.Second,
		},
		{
			name:          "Found after append",
			fileContents:  []string{"line1\nline2\n", "line3\n", "TEST2002\nline4\n"},
			writeInterval: 100 * time.Millisecond,
			pattern:       "TEST2002",
			shouldFind:    true,
			timeout:       1 * time.Second,
		},
		{
			name:          "Not found",
			fileContents:  []string{"line1\nline2\nline3\n", "line4\nline5\n"},
			writeInterval: 0,
			pattern:       "TEST3003",
			shouldFind:    false,
			timeout:       500 * time.Millisecond,
		},
		{
			name:          "Found in truncated file",
			fileContents:  []string{"line1\nline2\n", "", "TEST4004\n", "line5\n"},
			writeInterval: 100 * time.Millisecond,
			pattern:       "TEST4004",
			shouldFind:    true,
			timeout:       1 * time.Second,
		},
		{
			name:          "Pattern found across writes A",
			fileContents:  []string{"line1\nline2\nXXX", "ZZZ\nline3"},
			writeInterval: 100 * time.Millisecond,
			pattern:       "XXXZZZ",
			shouldFind:    true,
			timeout:       1 * time.Second,
		},
		{
			name:          "Pattern found across writes B",
			fileContents:  []string{"line1\nline2\nYYY", " WWW\nline3"},
			writeInterval: 100 * time.Millisecond,
			pattern:       "YYY WWW",
			shouldFind:    true,
			timeout:       1 * time.Second,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			is := is.New(t)
			// Create a test file
			filePath := filepath.Join(tmpDir, tc.name+".txt")
			file, err := os.Create(filePath)
			is.NoErr(err)
			file.Close()

			// Setup context with timeout
			ctx, cancel := context.WithTimeout(context.Background(), tc.timeout)
			defer cancel()

			got := false
			wg := sync.WaitGroup{}
			wg.Add(1)

			// Start the monitor in a goroutine.
			go func(x *bool) {
				defer wg.Done()
				f, err := os.Open(filePath)
				if err != nil {
					return
				}
				defer f.Close()
				found, err := monitorFileFor(ctx, f, tc.pattern)
				if err != nil {
					t.Logf("monitorFileFor returned an error: %v", err)
				}
				if found {
					t.Logf("monitorFileFor find result: %v", found)
					*x = found
				}
			}(&got)

			// Write to the file at specified interval.
			for _, content := range tc.fileContents {
				time.Sleep(tc.writeInterval)
				if len(content) == 0 {
					// Empty content means truncate the file.
					os.Truncate(filePath, 0)
					continue
				}
				f, err := os.OpenFile(filePath, os.O_WRONLY|os.O_APPEND, 0o644)
				is.NoErr(err)
				_, err = f.WriteString(content)
				is.NoErr(err)
				f.Close()
			}

			// TODO: If we want a timeout here we can create and
			// additional channel, move the `Wait` to another gorouting
			// and combine that channel in a select with a `time.After`.
			wg.Wait()

			is.Equal(tc.shouldFind, got)
		})
	}
}
