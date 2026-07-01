// Package result holds the Result type produced by command execution. It lives
// in its own leaf package (depending only on terraform-plugin-framework/diag)
// so that both internal/cmd and internal/exec can use it without creating an
// import cycle: internal/exec's LocalExecutor imports internal/cmd, while its
// SSHExecutor produces the same Result type independently.
package result

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
)

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
	PID           int // populated by RunDetached with the child process PID
}

func (r Result) Diagnostics() diag.Diagnostics {
	dz := diag.Diagnostics{}

	dz.AddError(fmt.Sprintf("%s exit code: %d", r.Cmd, r.ExitCode),
		fmt.Sprintf("args: %v", r.Args))
	dz.AddError(fmt.Sprintf("%s stdout", r.Cmd), r.Stdout)
	dz.AddError(fmt.Sprintf("%s stderr", r.Cmd), r.Stderr)

	return dz
}
