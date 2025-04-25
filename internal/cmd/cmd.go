package cmd

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
)

// NowFunc defines a function that returns a time.Time .
type NowFunc func() time.Time

// Now is the package level function used to get the current time. It can be
// overwritten for testing purposes.
var Now NowFunc = time.Now

// Result encapsulates the result of running a shell command including the exit
// code, stdout and stderr outputs and any error or timeout.
type Result struct {
	Cmd      string
	Args     []string
	ExitCode int
	Stdout   string
	Stderr   string
	Error    error
	Logs     struct {
		Stdout string
		Stderr string
	}
	MatchedString string
	Completed     bool
	TimedOut      bool
}

func (r Result) Diagnostics() diag.Diagnostics {
	dz := diag.Diagnostics{}

	dz.AddError(fmt.Sprintf("%s exit code: %d", r.Cmd, r.ExitCode),
		fmt.Sprintf("args: %v", r.Args))
	dz.AddError(fmt.Sprintf("%s stdout", r.Cmd), r.Stdout)
	dz.AddError(fmt.Sprintf("%s stderr", r.Cmd), r.Stderr)

	return dz
}

// safeBuffer is a simple wrapper around bytes.Buffer that is safe for
// concurrent use.
type safeBuffer struct {
	mu  sync.Mutex
	buf []byte
}

func (b *safeBuffer) Write(p []byte) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	return len(p), nil
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.buf)
}

func (b *safeBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	result := make([]byte, len(b.buf))
	copy(result, b.buf)
	return result
}

func CopyFile(src, dst string) (int64, error) {
	sourceFile, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return 0, err
	}
	defer destFile.Close()

	return io.Copy(destFile, sourceFile)
}
