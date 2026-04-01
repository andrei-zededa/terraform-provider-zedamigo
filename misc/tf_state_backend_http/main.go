package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// absStateDir is the resolved absolute path to the state directory.
// Set once at startup and used by handlers for path validation.
var absStateDir string

const maxBodySize = 10 << 20 // 10 MB

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)

	listenAddr := getEnv("TF_BACKEND_ADDR", "192.168.192.168:9000")
	stateDir := getEnv("TF_BACKEND_STATE_DIR", "./terraform_states")

	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		slog.Error("Failed to create root state directory", "error", err)
		os.Exit(1)
	}

	var err error
	absStateDir, err = filepath.Abs(stateDir)
	if err != nil {
		slog.Error("Failed to resolve absolute state directory path", "error", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", stateHandler)

	srv := &http.Server{
		Addr:    listenAddr,
		Handler: mux,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		slog.Info("Shutdown signal received, draining connections...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("Shutdown error", "error", err)
		}
	}()

	slog.Info("Starting Terraform HTTP state backend", "addr", listenAddr, "state_dir", absStateDir)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		slog.Error("Server failed to start", "error", err)
		os.Exit(1)
	}
	slog.Info("Server stopped")
}

func stateHandler(w http.ResponseWriter, r *http.Request) {
	cleanPath := filepath.Clean(r.URL.Path)
	if cleanPath == "/" || cleanPath == "." || cleanPath == "" {
		cleanPath = "/default.tfstate"
	}

	fullPath := filepath.Join(absStateDir, cleanPath)

	// Ensure the resolved path stays within the state directory.
	if !strings.HasPrefix(fullPath, absStateDir+string(os.PathSeparator)) && fullPath != absStateDir {
		slog.Warn("Path escapes state directory", "path", r.URL.Path, "resolved", fullPath, "ip", r.RemoteAddr)
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	slog.Debug("Incoming request", "method", r.Method, "url_path", r.URL.Path, "mapped_file", fullPath)

	switch r.Method {
	case http.MethodGet:
		handleGet(w, fullPath)
	case http.MethodPost, http.MethodPut:
		handlePost(w, r, fullPath)
	case http.MethodDelete:
		handleDelete(w, fullPath)
	case "LOCK", "UNLOCK":
		slog.Warn("Lock/Unlock stubbed -- no actual locking is performed", "method", r.Method, "path", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	default:
		slog.Warn("Unsupported method requested", "method", r.Method, "path", r.URL.Path)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleGet(w http.ResponseWriter, path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Info("State file not found (likely first run)", "path", path)
			http.Error(w, "State not found", http.StatusNotFound)
			return
		}
		slog.Error("Failed to read state file", "path", path, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	slog.Info("State served successfully", "path", path, "bytes", len(data))
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func handlePost(w http.ResponseWriter, r *http.Request, path string) {
	defer r.Body.Close()

	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Error("Failed to read request body", "error", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		slog.Error("Failed to create subdirectories", "path", filepath.Dir(path), "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if err := os.WriteFile(path, body, 0o644); err != nil {
		slog.Error("Failed to write state file", "path", path, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	slog.Info("State saved successfully", "path", path, "bytes", len(body))
	w.WriteHeader(http.StatusOK)
}

func handleDelete(w http.ResponseWriter, path string) {
	err := os.Remove(path)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Info("State file to delete was not found", "path", path)
			w.WriteHeader(http.StatusOK)
			return
		}
		slog.Error("Failed to delete state file", "path", path, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	slog.Info("State deleted successfully", "path", path)
	w.WriteHeader(http.StatusOK)
}
