package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

// Run executes a command and returns the result. It saves the command stdout
// and stderr output to files in the specified logPath.
func Run(logPath string, command string, args ...string) (Result, error) {
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

	cmd := exec.Command(command, args...)

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
	go func() {
		mw := io.MultiWriter(outFile, stdoutBuf)
		io.Copy(mw, stdoutPipe)
	}()
	stderrBuf := &safeBuffer{}
	go func() {
		mw := io.MultiWriter(errFile, stderrBuf)
		io.Copy(mw, stderrPipe)
	}()

	// Wait for the command to finish.
	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		}
		result.Error = err
	}
	result.Stdout = stdoutBuf.String()
	result.Stderr = stderrBuf.String()

	return result, result.Error
}

// RunBG executes a command "in the background" (asynchronously) and returns a
// channel that will receive the result when the command completes.
func RunBG(logPath string, command string, args ...string) <-chan Result {
	resultChan := make(chan Result, 1)

	go func() {
		result, _ := Run(logPath, command, args...)
		resultChan <- result
		close(resultChan)
	}()

	return resultChan
}

// RunDetached executes a command that will continue to run even when the current
// process exists.
func RunDetached(logPath string, command string, args ...string) (Result, error) {
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
	errFile, err := os.Create(stderrFile)
	if err != nil {
		outFile.Close()
		result.Error = fmt.Errorf("failed to create stderr file: %w", err)
		return result, result.Error
	}

	if err := os.WriteFile(filepath.Join(logPath, fmt.Sprintf("%s_%s_command.log", timestamp, cmdName)),
		[]byte(fmt.Sprintf("command=%s args=%v\n", command, args)),
		0o600); err != nil {
		result.Error = fmt.Errorf("failed to create command log file: %w", err)
		return result, result.Error
	}

	cmd := exec.Command(command, args...)
	cmd.Stdout = outFile
	cmd.Stderr = errFile

	// Set process group ID to detach from parent.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	// Start the command.
	if err := cmd.Start(); err != nil {
		outFile.Close()
		errFile.Close()
		result.Error = fmt.Errorf("failed to start command: %w", err)
		return result, result.Error
	}

	// Wait a bit to give time to the command to write something to it's
	// stdout/stderr if it wants. TODO: It is unclear if the FD's are
	// closed when the current process exists, therefore it is unclear if
	// the detached command can still write to them afterwards.
	<-time.After(250 * time.Millisecond)

	outFile.Sync()
	errFile.Sync()

	return result, result.Error
}
