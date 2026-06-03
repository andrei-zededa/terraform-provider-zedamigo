package cmd_test

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/cmd"
)

// TestRunCapturesFullOutput guards against an output-capture race where Run
// could return before the command's stdout had been fully drained, yielding
// empty or truncated Stdout. It runs a command that emits a large, known
// payload many times and asserts the captured output is complete every time.
func TestRunCapturesFullOutput(t *testing.T) {
	logPath := t.TempDir()

	const nLines = 5000
	var want strings.Builder
	for i := 1; i <= nLines; i++ {
		fmt.Fprintf(&want, "line %d\n", i)
	}
	script := fmt.Sprintf("for i in $(seq 1 %d); do echo line $i; done", nLines)

	for iter := 0; iter < 200; iter++ {
		result, err := cmd.Run(logPath, "bash", "-c", script)
		if err != nil {
			t.Fatalf("iter %d: Run returned error: %v (stderr: %q)", iter, err, result.Stderr)
		}
		if result.Stdout != want.String() {
			t.Fatalf("iter %d: incomplete stdout: got %d bytes, want %d bytes",
				iter, len(result.Stdout), want.Len())
		}
	}
}

// TestRunConcurrentCapture exercises the same guarantee under concurrency,
// which is how the provider calls Run (Terraform applies resources in
// parallel). Each goroutine must receive its own complete output.
func TestRunConcurrentCapture(t *testing.T) {
	logPath := t.TempDir()

	const nLines = 2000
	var want strings.Builder
	for i := 1; i <= nLines; i++ {
		fmt.Fprintf(&want, "line %d\n", i)
	}
	script := fmt.Sprintf("for i in $(seq 1 %d); do echo line $i; done", nLines)

	var wg sync.WaitGroup
	errs := make(chan error, 50)
	for g := 0; g < 50; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			result, err := cmd.Run(logPath, "bash", "-c", script)
			if err != nil {
				errs <- fmt.Errorf("goroutine %d: Run error: %w", g, err)
				return
			}
			if result.Stdout != want.String() {
				errs <- fmt.Errorf("goroutine %d: incomplete stdout: got %d bytes, want %d bytes",
					g, len(result.Stdout), want.Len())
			}
		}(g)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}
