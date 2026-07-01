package cmd

import (
	"io"
	"os"
	"sync"
	"time"

	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/cmd/result"
)

// NowFunc defines a function that returns a time.Time .
type NowFunc func() time.Time

// Now is the package level function used to get the current time. It can be
// overwritten for testing purposes.
var Now NowFunc = time.Now

// Result is an alias for result.Result. The type was moved to the leaf package
// internal/cmd/result to break a potential import cycle with internal/exec
// (whose SSHExecutor must produce the same Result without importing this
// package). The alias keeps all existing cmd.Result references compiling.
type Result = result.Result

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
