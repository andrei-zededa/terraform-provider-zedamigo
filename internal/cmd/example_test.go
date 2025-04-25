package cmd_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/cmd"
)

// ExampleRun demonstrates how to synchronously execute a command.
func ExampleRun() {
	cmd.Now = func() time.Time {
		return time.Date(2003, 1, 1, 0, 0, 0, 0, time.UTC)
	}

	// Create a temporary log directory for the example.
	logPath := "./testlogs"
	os.MkdirAll(logPath, 0o700)
	defer os.RemoveAll(logPath) // Clean up after the example.

	result, err := cmd.Run(logPath, "echo", "Hello, World!")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Exit code: %d\n", result.ExitCode)
	fmt.Printf("Stdout: %s\n", result.Stdout)
	fmt.Printf("Log files created at: %s and %s\n", result.Logs.Stdout, result.Logs.Stderr)

	// Output:
	// Exit code: 0
	// Stdout: Hello, World!
	//
	// Log files created at: testlogs/20030101_000000_echo_stdout.log and testlogs/20030101_000000_echo_stderr.log
}

// ExampleRunBG demonstrates how to asynchronously execute a command.
func ExampleRunBG() {
	// Create a temporary log directory for the example.
	logPath := "./testlogs"
	os.MkdirAll(logPath, 0o700)
	defer os.RemoveAll(logPath) // Clean up after the example.

	// Start a command asynchronously.
	resultChan := cmd.RunBG(logPath, "echo", "Async command")

	// Do other work while the command is running.
	fmt.Println("Command is running in the background...")

	// Wait for and process the result.
	result := <-resultChan

	fmt.Printf("Exit code: %d\n", result.ExitCode)
	fmt.Printf("Stdout: %s\n", result.Stdout)

	// Output:
	// Command is running in the background...
	// Exit code: 0
	// Stdout: Async command
}

// ExampleRun_multi demonstrates running and collecting results
// from multiple asynchronous commands.
func ExampleRun_multi() {
	// Create a temporary log directory for the example.
	logPath := "./testlogs"
	os.MkdirAll(logPath, 0o700)
	defer os.RemoveAll(logPath) // Clean up after the example.

	// Start multiple commands asynchronously.
	cmd1Chan := cmd.RunBG(logPath, "/bin/sh", "-c", "sleep 3; echo 'First command'")
	cmd2Chan := cmd.RunBG(logPath, "echo", "Second command")

	// Create a map to store results as they arrive.
	results := make(map[string]string)

	// Wait for both commands to complete.
	for i := 0; i < 2; i++ {
		select {
		case result := <-cmd1Chan:
			results["cmd1"] = result.Stdout
			cmd1Chan = nil // Prevent selecting this channel again.

		case result := <-cmd2Chan:
			results["cmd2"] = result.Stdout
			cmd2Chan = nil
		}
	}

	// Print results in a deterministic order for testing.
	fmt.Printf("Command 1: %s\n", results["cmd1"])
	fmt.Printf("Command 2: %s\n", results["cmd2"])

	// Output:
	// Command 1: First command
	//
	// Command 2: Second command
}

// ExampleRunMatch demonstrates waiting for a specific pattern in
// the command output.
func ExampleRunMatch() {
	// Create a temporary log directory for the example.
	logPath := "./testlogs"
	os.MkdirAll(logPath, 0o700)
	defer os.RemoveAll(logPath) // Clean up after the example.

	// Run a command and wait for a specific pattern to appear in its output.
	result, err := cmd.RunMatch(
		logPath,       // Log path
		"6",           // Pattern to wait for
		5*time.Second, // Timeout, should be > 9*0.5
		"bash",        // Command
		"-c",          // Argument
		"for i in {1..9}; do echo $i; sleep 0.5; done", // Bash script.
	)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Pattern '%s' found: %v\n", "6", result.MatchedString != "")
	fmt.Printf("Command completed normally: %v\n", result.Completed)
	fmt.Printf("Command timed out: %v\n", result.TimedOut)

	// Print the command stdout line by line.
	for i, line := range strings.Split(result.Stdout, "\n") {
		if line != "" {
			fmt.Printf("[line #%d]: %s\n", i+1, line)
		}
	}

	// Output:
	// Pattern '6' found: true
	// Command completed normally: false
	// Command timed out: false
	// [line #1]: 1
	// [line #2]: 2
	// [line #3]: 3
	// [line #4]: 4
	// [line #5]: 5
	// [line #6]: 6
}

// ExampleRunMatchBG demonstrates asynchronously waiting for a pattern with
// timeout handling.
func ExampleRunMatchBG() {
	// Create a temporary log directory for the example.
	logPath := "./testlogs"
	os.MkdirAll(logPath, 0o700)
	defer os.RemoveAll(logPath) // Clean up after the example.

	// Run a command asynchronously with a pattern that won't be found
	// and a short timeout.
	patternChan := cmd.RunMatchBG(
		logPath,                    // Log path
		"pattern-that-wont-appear", // Pattern to wait for
		time.Second,                // Short timeout but still longer than it takes echo to execute.
		"echo",                     // Command
		"Hello, timeout example",   // Arguments
	)

	fmt.Println("Waiting for pattern match or timeout...")
	result := <-patternChan

	fmt.Printf("Timed out: %v\n", result.TimedOut)
	fmt.Printf("Pattern found: %v\n", result.MatchedString != "")
	fmt.Printf("Command completed: %v\n", result.Completed)
	fmt.Printf("Stdout: %s\n", result.Stdout)

	// Output:
	// Waiting for pattern match or timeout...
	// Timed out: false
	// Pattern found: false
	// Command completed: true
	// Stdout: Hello, timeout example
}

// ExampleRunMatchBG_actual_timeout demonstrates asynchronously waiting for a
// pattern with timeout handling.
func ExampleRunMatchBG_actual_timeout() {
	// Create a temporary log directory for the example.
	logPath := "./testlogs"
	os.MkdirAll(logPath, 0o700)
	defer os.RemoveAll(logPath) // Clean up after the example.

	// Run a command asynchronously with a pattern that won't be found
	// and a short timeout.
	patternChan := cmd.RunMatchBG(
		logPath,                    // Log path
		"pattern-that-wont-appear", // Pattern to wait for
		time.Second,                // Short timeout.
		"bash",                     // Command
		"-c",                       // Arguments
		"echo 'Hello, timeout example'; sleep 3",
	)

	fmt.Println("Waiting for pattern match or timeout...")
	result := <-patternChan

	fmt.Printf("Timed out: %v\n", result.TimedOut)
	fmt.Printf("Pattern found: %v\n", result.MatchedString != "")
	fmt.Printf("Command completed: %v\n", result.Completed)
	fmt.Printf("Stdout: %s\n", result.Stdout)

	// Output:
	// Waiting for pattern match or timeout...
	// Timed out: true
	// Pattern found: false
	// Command completed: false
	// Stdout: Hello, timeout example
}

// ExampleRunMatchFile demonstrates executing a command and waiting for a
// pattern to appear in a separate file.
func ExampleRunMatchFile() {
	// Create a temporary log directory for the example.
	logPath := "./testlogs"
	os.MkdirAll(logPath, 0o700)
	// defer os.RemoveAll(logPath) // Clean up after the example.

	// Create a temporary file to watch
	watchFilePath := filepath.Join(logPath, "watch_file.log")

	// Start a command that will write to the watched file.
	result, err := cmd.RunMatchFile(
		logPath,           // Log path
		"pattern-to-find", // Pattern to wait for
		watchFilePath,     // File to watch for the pattern
		5*time.Second,     // Timeout
		"bash",            // Command
		"-c",              // Arguments
		fmt.Sprintf("sleep 1; echo 'pattern-to-find' > %s; sleep 1;", watchFilePath),
		// NOTE: This can fail intermittently with != stdout due to timing.
		// fmt.Sprintf("echo 'Starting'; sleep 1; echo 'pattern-to-find' > %s; sleep 1; echo 'File generated'; sleep 1; echo 'Done'", watchFilePath),
	)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Pattern '%s' found: %v\n", "pattern-to-find", result.MatchedString != "")
	fmt.Printf("Command completed: %v\n", result.Completed)
	fmt.Printf("Command timed out: %v\n", result.TimedOut)
	fmt.Printf("Stdout: %s\n", result.Stdout)

	// Output:
	// Pattern 'pattern-to-find' found: true
	// Command completed: false
	// Command timed out: false
	// Stdout:
}

// ExampleRunMatchFileBG demonstrates asynchronously executing a command and
// waiting for a pattern to appear in a separate file.
func ExampleRunMatchFileBG() {
	// Create a temporary log directory for the example.
	logPath := "./testlogs"
	os.MkdirAll(logPath, 0o700)
	defer os.RemoveAll(logPath) // Clean up after the example.

	// Create a temporary file to watch.
	watchFilePath := filepath.Join(logPath, "watch_file_bg.log")

	// Start a command asynchronously that will write to the watched file.
	resultChan := cmd.RunMatchFileBG(
		logPath,           // Log path
		"pattern-to-find", // Pattern to wait for
		watchFilePath,     // File to watch for the pattern
		5*time.Second,     // Timeout
		"bash",            // Command
		"-c",              // Arguments
		fmt.Sprintf("sleep 1; echo 'pattern-to-find' > %s; sleep 1;", watchFilePath),
		// NOTE: See note above.
		// fmt.Sprintf("echo 'Starting BG command'; sleep 1; echo 'pattern-to-find' > %s; echo 'BG command finished'; sleep 1; echo 'Done'", watchFilePath),
	)

	fmt.Println("Waiting for pattern match in file...")
	result := <-resultChan

	fmt.Printf("Pattern found: %v\n", result.MatchedString != "")
	fmt.Printf("Command completed: %v\n", result.Completed)
	fmt.Printf("Command timed out: %v\n", result.TimedOut)
	fmt.Printf("Stdout: %s\n", result.Stdout)

	// Output:
	// Waiting for pattern match in file...
	// Pattern found: true
	// Command completed: false
	// Command timed out: false
	// Stdout:
}
