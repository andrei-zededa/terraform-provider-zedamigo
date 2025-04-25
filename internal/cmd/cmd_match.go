package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// RunMatch executes a command and waits until either:
//   - A specific string pattern appears in stdout or stderr.
//   - The timeout is reached.
//   - The command completes/exits.
func RunMatch(logPath string, waitPattern string, timeout time.Duration, command string, args ...string) (Result, error) {
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

	// Create a context with the specified timeout.
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Create the command with the context for timeout.
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

	// Channel to signal when the pattern is found.
	patternFound := make(chan struct{})
	// Channel to signal when the command completes.
	cmdDone := make(chan error, 1)

	// Scan stdout for the pattern.
	go func() {
		// Write to both the file and our buffer.
		mw := io.MultiWriter(outFile, stdoutBuf)

		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			line := scanner.Text()
			mw.Write([]byte(line + "\n"))

			// Check if line contains the pattern.
			if strings.Contains(line, waitPattern) {
				result.MatchedString = waitPattern
				select {
				case patternFound <- struct{}{}:
					// Signal sent.
				default:
					// Channel already closed or pattern already found.
				}
				return
			}

			// Check if context is done
			select {
			case <-ctx.Done():
				return
			default:
				// Continue scanning
			}
		}
	}()

	// Scan stderr for the pattern.
	go func() {
		// Write to both the file and our buffer.
		mw := io.MultiWriter(errFile, stderrBuf)

		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			line := scanner.Text()
			mw.Write([]byte(line + "\n"))

			// Check if line contains the pattern.
			if strings.Contains(line, waitPattern) {
				result.MatchedString = waitPattern
				select {
				case patternFound <- struct{}{}:
					// Signal sent.
				default:
					// Channel already closed or pattern already found.
				}
				return
			}

			// Check if context is done
			select {
			case <-ctx.Done():
				return
			default:
				// Continue scanning
			}
		}
	}()

	// Start a goroutine to wait for command completion.
	go func() {
		cmdDone <- cmd.Wait()
	}()

	// Wait for either:
	// 1. The pattern to be found
	// 2. The context to timeout
	// 3. The command to exit naturally
	select {
	case <-patternFound:
		// Pattern found, kill the process if it's still running.
		cmd.Process.Kill()
		result.Stdout = stdoutBuf.String()
		result.Stderr = stderrBuf.String()

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
	}

	return result, result.Error
}

// RunMatchBG is the same as RunMatch but executes "in the background"
// (asynchronously) and returns a channel that will receive the result when
// RunMatch finishes.
func RunMatchBG(logPath string, waitPattern string, timeout time.Duration, command string, args ...string) <-chan Result {
	resultChan := make(chan Result, 1)

	go func() {
		result, _ := RunMatch(logPath, waitPattern, timeout, command, args...)
		resultChan <- result
		close(resultChan)
	}()

	return resultChan
}
