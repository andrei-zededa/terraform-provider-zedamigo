package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// RunMatchFile executes a command and waits until either:
//   - A specific string pattern appears in the specified file.
//   - The timeout is reached.
//   - The command completes/exits.
func RunMatchFile(logPath string, waitPattern string, watchFilePath string, timeout time.Duration, command string, args ...string) (Result, error) {
	result := Result{
		Cmd:  command,
		Args: args,
	}

	// Create the log directory if it doesn't exist. Prepare the log files.
	if err := os.MkdirAll(logPath, 0o700); err != nil {
		result.Error = fmt.Errorf("failed to create log directory: %w", err)
		return result, result.Error
	}
	cmdName := filepath.Base(command)
	timestamp := Now().Format("20060102_150405")
	stdoutFile := filepath.Join(logPath, fmt.Sprintf("%s_%s_stdout.log", timestamp, cmdName))
	stderrFile := filepath.Join(logPath, fmt.Sprintf("%s_%s_stderr.log", timestamp, cmdName))
	result.Logs.Stdout = stdoutFile
	result.Logs.Stderr = stderrFile

	outFile, err := os.Create(stdoutFile)
	if err != nil {
		result.Error = fmt.Errorf("failed to create stdout file: %w", err)
		return result, result.Error
	}
	defer outFile.Close()
	errFile, err := os.Create(stderrFile)
	if err != nil {
		result.Error = fmt.Errorf("failed to create stderr file: %w", err)
		return result, result.Error
	}
	defer errFile.Close()

	if err := os.WriteFile(filepath.Join(logPath, fmt.Sprintf("%s_%s_command.log", timestamp, cmdName)),
		[]byte(fmt.Sprintf("command=%s args=%v\n", command, args)),
		0o600); err != nil {
		result.Error = fmt.Errorf("failed to create command log file: %w", err)
		return result, result.Error
	}

	// Create a context with the specified timeout.
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, command, args...)

	// Create pipes for capturing output.
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		result.Error = fmt.Errorf("failed to create stdout pipe: %w", err)
		return result, result.Error
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		result.Error = fmt.Errorf("failed to create stderr pipe: %w", err)
		return result, result.Error
	}

	// Start the command.
	if err := cmd.Start(); err != nil {
		result.Error = fmt.Errorf("failed to start command: %w", err)
		return result, result.Error
	}
	stdoutBuf := &safeBuffer{}
	stderrBuf := &safeBuffer{}

	// Channel to signal when the command completes.
	cmdDone := make(chan error, 1)
	// Start a goroutine to wait for command completion.
	go func() {
		cmdDone <- cmd.Wait()
	}()

	// Copy stdout to the log file and the buffer.
	go func() {
		mw := io.MultiWriter(outFile, stdoutBuf)
		io.Copy(mw, stdoutPipe)
	}()

	// Copy stderr to the log file and the buffer.
	go func() {
		mw := io.MultiWriter(errFile, stderrBuf)
		io.Copy(mw, stderrPipe)
	}()

	// Wait for the file to exist.
	for {
		if _, err := os.Stat(watchFilePath); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			if ctx.Err() == context.DeadlineExceeded {
				// Timeout reached.
				cmd.Process.Kill()
				result.TimedOut = true
				result.Error = fmt.Errorf("command timed out after %v", timeout)
			}
			result.Stdout = stdoutBuf.String()
			result.Stderr = stderrBuf.String()

			return result, result.Error
		default:
			time.Sleep(100 * time.Millisecond)
		}
	}

	// Monitor the file for the pattern.
	f, err := os.Open(watchFilePath)
	if err != nil {
		result.Error = err
		return result, result.Error
	}
	found, err := monitorFileFor(ctx, f, waitPattern)
	if err != nil {
		result.Error = err
		return result, result.Error
	}

	// Wait for either:
	// 1. The pattern to be found in the watched file
	// 2. The context to timeout
	// 3. The command to exit naturally
	select {
	case <-ctx.Done():
		if ctx.Err() == context.DeadlineExceeded {
			// Timeout reached.
			cmd.Process.Kill()
			result.TimedOut = true
			result.Error = fmt.Errorf("command timed out after %v", timeout)
		}
		result.Stdout = stdoutBuf.String()
		result.Stderr = stderrBuf.String()

	case err := <-cmdDone:
		// Command exited.
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				result.ExitCode = exitErr.ExitCode()
			}
			result.Error = err
		}
		result.Completed = true
		result.Stdout = stdoutBuf.String()
		result.Stderr = stderrBuf.String()
	default:
		if found {
			// Pattern found.
			result.MatchedString = waitPattern
			// Kill the process if it's still running.
			cmd.Process.Kill()
			result.Completed = false // Explicitly set to false since we're killing the process.
			result.Stdout = stdoutBuf.String()
			result.Stderr = stderrBuf.String()

			break
		}
	}

	return result, result.Error
}

// RunMatchFileBG is the same as RunMatchFile but executes "in the background"
// (asynchronously) and returns a channel that will receive the result when
// RunMatchFile finishes.
func RunMatchFileBG(logPath string, waitPattern string, watchFilePath string, timeout time.Duration, command string, args ...string) <-chan Result {
	resultChan := make(chan Result, 1)

	go func() {
		result, _ := RunMatchFile(logPath, waitPattern, watchFilePath, timeout, command, args...)
		resultChan <- result
		close(resultChan)
	}()

	return resultChan
}
