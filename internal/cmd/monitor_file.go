package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"time"
)

const (
	colorYellow     = "\033[33m"
	colorBrightRed  = "\033[91m"
	colorBrightCyan = "\033[96m"
	colorReset      = "\033[0m"
)

// monitorFileFor watches a file for a specific pattern, handling file truncation
// and growth appropriately. It uses direct file reads instead of a scanner.
// monitorFileFor will return when either:
//   - The pattern is found: true, nil.
//   - The context is canceled: false, err.
//   - There is an error reading from the either other than EOF: false, err.
func monitorFileFor(ctx context.Context, f *os.File, lookForStr string) (bool, error) {
	// Create buffers for reading and processing data, any additional things
	// needed.
	lookFor := []byte(lookForStr)
	maxLen := 64
	prev := make([]byte, maxLen)
	prev_n := 0
	curr := make([]byte, maxLen)
	combined := make([]byte, 2*maxLen)
	pos := int64(0)

	// fmt.Fprintf(os.Stderr, "Looking for:\n%s%s%s", colorBrightCyan, hex.Dump(lookFor), colorReset)

	for {
		// Check if the context is already done.
		select {
		case <-ctx.Done():
			return false, fmt.Errorf("monitorFileFor: context canceled: %w", ctx.Err())
		default:
			// Continue monitoring.
		}

		// Check the current file size.
		fi, err := f.Stat()
		if err != nil {
			return false, fmt.Errorf("monitorFileFor: file stat failed: %w", err)
		}
		currSize := fi.Size()
		if currSize == pos {
			// File hasn't grown, wait a bit and check again.
			// fmt.Fprintf(os.Stderr, "File hasn't grown, wait a bit and check again.\n")
			time.Sleep(100 * time.Millisecond)
			continue
		}
		if currSize < pos {
			// File was truncated, start from beginning.
			// fmt.Fprintf(os.Stderr, "File was truncated, start from beginning.\n")
			p, err := f.Seek(0, io.SeekStart)
			if err != nil {
				return false, fmt.Errorf("monitorFileFor: file seek (to start after truncation) failed: %w", err)
			}
			pos = p
			prev_n = 0
		}

		// Read new content.
		n, err := f.Read(curr)
		if err != nil && err != io.EOF {
			return false, fmt.Errorf("monitorFileFor: read failed: %w", err)
		}
		if n == 0 {
			// There is no more data (we reached EOF ?) wait a bit
			// before trying again.
			// fmt.Fprintf(os.Stderr, "There is no more data (we reached EOF ?) wait a bit before trying again.\n")
			time.Sleep(100 * time.Millisecond)
			continue
		}
		// Update position.
		pos += int64(n)
		// fmt.Fprintf(os.Stderr, "Read %d=%d bytes:\n%s%s%s", n, len(curr), colorYellow, hex.Dump(curr), colorReset)

		if prev_n > 0 {
			copy(combined, prev[:prev_n])
		}
		copy(combined[prev_n:], curr[:n])
		// fmt.Fprintf(os.Stderr, "Combined %d bytes:\n%s%s%s", len(combined), colorYellow, hex.Dump(combined), colorReset)
		if bytes.Contains(combined, lookFor) {
			// fmt.Fprintf(os.Stderr, "Look for %s%s%s!\n", colorBrightRed, "found", colorReset)
			return true, nil
		}

		// Save the currently read buffer.
		copy(prev, curr[:n])
		prev_n = n
		// fmt.Fprintf(os.Stderr, "Prev %d bytes:\n%s%s%s", prev_n, colorYellow, hex.Dump(prev), colorReset)
	}
}

// intoLines splits a byte array into lines, handling different line endings.
// Returns at least 1 line, which will be the entire input in casse there is no
// actual newline in it.
func intoLines(data []byte) []string {
	result := []string{}

	if len(data) == 0 {
		return result
	}

	data = append(data, []byte("\n")...)
	data = bytes.ReplaceAll(data, []byte("\r"), []byte("\n"))

	// Split into lines and remove empty ones.
	for _, line := range bytes.Split(data, []byte("\n")) {
		if len(line) > 0 {
			result = append(result, string(line))
		}
	}

	return result
}
