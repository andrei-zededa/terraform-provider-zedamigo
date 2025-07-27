package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
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
		go handleConnection(ctx, conn, t)
	}
}

func handleConnection(ctx context.Context, conn net.Conn, t *Tailer) {
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
		return fmt.Errorf("socket file does not exist: %s", socketPath)
	}

	// Connect to the UNIX socket.
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return fmt.Errorf("failed to connect to UNIX socket %s: %w", socketPath, err)
	}
	defer conn.Close()

	t.logInfo("Connected to UNIX socket", "path", socketPath)

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

func main() {
	var (
		listenPath  = flag.String("listen", "", "Listen on UNIX socket at given path")
		connectPath = flag.String("connect", "", "Connect to existing UNIX socket at given path")
		outputFile  = flag.String("out", "", "Output file (default: stdout)")
		verbose     = flag.Bool("v", false, "Verbose logging")
	)
	flag.Parse()

	// Set up a new logger.
	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	}))
	slog.SetDefault(logger)

	// Validate CLI flags.
	if *listenPath == "" && *connectPath == "" {
		fmt.Fprintf(os.Stderr, "Error: Must specify either -listen or -connect\n")
		flag.Usage()
		os.Exit(1)
	}

	if *listenPath != "" && *connectPath != "" {
		fmt.Fprintf(os.Stderr, "Error: Cannot specify both -listen and -connect\n")
		flag.Usage()
		os.Exit(1)
	}

	// Determine mode and socket path.
	var mode, socketPath string
	if *listenPath != "" {
		mode = "listen"
		socketPath = *listenPath
	} else {
		mode = "connect"
		socketPath = *connectPath
	}

	var w io.Writer = os.Stdout
	var outputFileHandle *os.File
	if *outputFile != "" {
		file, err := os.OpenFile(*outputFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			logger.Error("Failed to open output file", "file", *outputFile, "error", err)
			os.Exit(1)
		}
		outputFileHandle = file
		w = file
		logger.Info("Writing to file", "file", *outputFile)
	} else {
		logger.Info("Writing to stdout")
	}

	tailer := NewTailer(w, logger)

	// Set up a context for graceful shutdown.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		logger.Info("Received shutdown signal")
		cancel()
	}()

	// Run and handle cleanup
	var err error
	if mode == "listen" {
		err = tailer.RunServer(ctx, socketPath)
	} else {
		err = tailer.RunClient(ctx, socketPath)
	}

	// Cleanup
	if closeErr := tailer.Close(); closeErr != nil {
		logger.Error("Failed to close logger", "error", closeErr)
	}

	if outputFileHandle != nil {
		if closeErr := outputFileHandle.Close(); closeErr != nil {
			logger.Error("Failed to close output file", "error", closeErr)
		}
	}

	if err != nil {
		logger.Error("Operation failed", "mode", mode, "error", err)
		os.Exit(1)
	}
}
