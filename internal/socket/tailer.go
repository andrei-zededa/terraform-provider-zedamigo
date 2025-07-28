// Package socket provides helpers for working with UNIX sockets. Currently
// only the `Tailer` is implemented.
package socket

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"sync"
	"time"
)

const (
	timestampFmt = "2006-01-02 15:04:05.000"
	syncEvery    = 100 // syncEvery is the number of written lines after which to call Sync.
)

// Now is the function used to retrieve the current time(stamp). Equal to
// `time.Now` by default but can be overwritten for testing purposes.
var Now = time.Now

// Tailer  will write lines of text with a timestamp.
type Tailer struct {
	log    *slog.Logger
	isFile bool
	f      *os.File
	mutex  sync.Mutex
	w      io.Writer
	closed bool
	gauge  int
}

// WriteLine writes a line of text together with a timestamp.
func (t *Tailer) WriteLine(line string) error {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if t.closed {
		return fmt.Errorf("tailer is closed")
	}

	withTimestamp := "[" + Now().Format(timestampFmt) + "] " + line + "\n"

	if n, err := t.w.Write([]byte(withTimestamp)); err != nil {
		return fmt.Errorf("tailer failed to write line: %w", err)
	} else {
		if n != len(withTimestamp) {
			return fmt.Errorf("tailer partial write (%d out of %d bytes)",
				n, len(withTimestamp))
		}
	}

	if !t.isFile {
		return nil
	}

	if t.gauge > 0 {
		t.gauge--
		return nil
	}

	if err := t.f.Sync(); err != nil {
		return fmt.Errorf("file sync failed: %w", err)
	}

	t.gauge = syncEvery
	return nil
}

func (t *Tailer) logDebug(msg string, args ...any) {
	if t.log == nil {
		return
	}

	t.log.Debug(msg, args...)
}

func (t *Tailer) logInfo(msg string, args ...any) {
	if t.log == nil {
		return
	}

	t.log.Info(msg, args...)
}

func (t *Tailer) logError(msg string, args ...any) {
	if t.log == nil {
		return
	}

	t.log.Error(msg, args...)
}

// Close closes the file if the underlying writer is a file.
func (t *Tailer) Close() error {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if t.closed {
		return nil
	}

	if !t.isFile {
		t.closed = true
		return nil
	}

	if err := t.f.Sync(); err != nil {
		t.logError("Failed to sync file before close", "error", err)
	}

	if err := t.f.Close(); err != nil {
		// NOTE: Why should be mark the tailer/file as closed even when
		// the close call failed ?
		// t.closed = true
		return fmt.Errorf("close failed: %w", err)
	}

	t.closed = true
	return nil
}

// NewTailer creates a new Trailer from an io.Writer which might be a file.
func NewTailer(writer io.Writer, logger *slog.Logger) *Tailer {
	t := &Tailer{
		log: logger,
		w:   writer,
	}
	if file, ok := t.w.(*os.File); ok && file != os.Stdout {
		t.isFile = true
		t.f = file
		t.gauge = syncEvery
	}

	return t
}

// RunServer will run a server listening on a UNIX socket and with log each line
// to it's writer.
func (t *Tailer) RunServer(ctx context.Context, socketPath string) error {
	// Remove any existing socket file.
	if err := os.RemoveAll(socketPath); err != nil {
		return fmt.Errorf("failed to remove existing socket: %w", err)
	}

	// Listen on the UNIX socket.
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("failed to listen on UNIX socket: %w", err)
	}
	defer func() {
		listener.Close()
		os.RemoveAll(socketPath)
	}()

	// Set socket permissions such as that only the owner can connect.
	if err := os.Chmod(socketPath, 0o600); err != nil {
		return fmt.Errorf("failed to set permissions on UNIX socket: %w", err)
	}

	t.logInfo("Listening on UNIX socket", "path", socketPath)

	// Close the listener when the context is cancelled. This will also
	// cause Accept() to unblock and return with an error.
	go func() {
		<-ctx.Done()
		t.logInfo("Server shutting down")
		listener.Close() // NOTE: Probably redundant with the defer func.
	}()

	// Accept connections.
	for {
		conn, err := listener.Accept()
		if err != nil {
			// Check if the listener was closed due to shutdown.
			if opErr, ok := err.(*net.OpError); ok && opErr.Err.Error() == "use of closed network connection" {
				return nil
			}

			t.logError("Failed to accept connection", "error", err)
			continue // Keep trying to accept connections.
		}

		// Handle each connection in a separate goroutine.
		go handleConnection(conn, t)
	}
}

func handleConnection(conn net.Conn, t *Tailer) {
	defer conn.Close()

	clientAddr := conn.RemoteAddr()
	t.logInfo("New connection", "addr", clientAddr)

	scanner := bufio.NewScanner(conn)
	scanner.Split(bufio.ScanLines)

	for scanner.Scan() {
		line := scanner.Text()

		err := t.WriteLine(line)
		if err != nil {
			t.logError("Failed to write line", "error", err)
			continue
		}

		t.logDebug("Logged line", "addr", clientAddr, "line", line)
	}

	if err := scanner.Err(); err != nil {
		t.logError("Error reading from connection", "addr", clientAddr, "error", err)
		return
	}

	t.logInfo("Connection closed", "addr", clientAddr)
}

// RunClient will try to connect to an existing socket and read lines from that
// socket and write them to the it's writer.
func (t *Tailer) RunClient(ctx context.Context, socketPath string) error {
	// Check if the socket exists.
	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		t.logInfo("Socket file does not exist", "path", socketPath, "error", err)
	}

	// Connect to the UNIX socket.
	var conn net.Conn
	for {
		c, err := net.Dial("unix", socketPath)
		if err != nil {
			t.logError("Failed to connect to UNIX socket, will retry", "path", socketPath, "error", err)
			time.Sleep(337 * time.Millisecond)
			continue
		}
		defer c.Close()

		conn = c
		t.logInfo("Connected to UNIX socket", "path", socketPath)
		break
	}

	// Close the connection when the context is cancelled.
	go func() {
		<-ctx.Done()
		t.logInfo("Client shutting down")
		conn.Close() // NOTE: Probably redundant with the defer call.
	}()

	// Read lines from the socket.
	scanner := bufio.NewScanner(conn)
	scanner.Split(bufio.ScanLines)

	for scanner.Scan() {
		line := scanner.Text()

		if err := t.WriteLine(line); err != nil {
			t.logError("Failed to write line", "error", err)
			continue
		}

		t.logDebug("Logged line", "line", line)
	}

	if err := scanner.Err(); err != nil {
		// Check if the connection was closed due to shutdown.
		if opErr, ok := err.(*net.OpError); ok && opErr.Err.Error() == "use of closed network connection" {
			return nil
		}

		return fmt.Errorf("error reading from socket: %w", err)
	}

	t.logInfo("Socket connection closed")
	return nil
}
