package errchecker

import (
	"errors"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
)

type multiWrapper interface {
	Unwrap() []error
}

// ContainsAny searches through an error and all its wrapped errors to check
// if any error message contains any of the provided search strings.
func ContainsAny(err error, searchStrings []string) bool {
	if err == nil || len(searchStrings) == 0 {
		return false
	}

	// Use a map to track visited errors to avoid infinite loops.
	visited := make(map[error]bool)

	// Queue for BFS through error tree.
	queue := []error{err}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		// Skip if we've already visited this error.
		if visited[current] {
			continue
		}
		visited[current] = true

		// Check if current error contains any of the search strings.
		errMsg := current.Error()
		for _, searchStr := range searchStrings {
			if strings.Contains(errMsg, searchStr) {
				return true
			}
		}

		// Handle single wrapped error.
		if unwrapped := errors.Unwrap(current); unwrapped != nil {
			queue = append(queue, unwrapped)
		}

		// Handle multiple wrapped errors (e.g., from errors.Join).
		if mw, ok := current.(multiWrapper); ok {
			queue = append(queue, mw.Unwrap()...)
		}
	}

	return false
}

// ContainsNone searches through an error and all its wrapped errors to check
// if any error message contains any of the provided search strings. It returns
// `true` if none of the errors contain any of the strings.
func ContainsNone(err error, searchStrings []string) bool {
	if err == nil || len(searchStrings) == 0 {
		return true
	}

	// Use a map to track visited errors to avoid infinite loops.
	visited := make(map[error]bool)

	// Queue for BFS through error tree.
	queue := []error{err}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		// Skip if we've already visited this error.
		if visited[current] {
			continue
		}
		visited[current] = true

		// Check if current error contains any of the search strings.
		errMsg := current.Error()
		for _, searchStr := range searchStrings {
			if strings.Contains(errMsg, searchStr) {
				return false
			}
		}

		// Handle single wrapped error.
		if unwrapped := errors.Unwrap(current); unwrapped != nil {
			queue = append(queue, unwrapped)
		}

		// Handle multiple wrapped errors (e.g., from errors.Join).
		if mw, ok := current.(multiWrapper); ok {
			queue = append(queue, mw.Unwrap()...)
		}
	}

	return true
}

// DiagsAny searches through a list of Diagnostics to check if any of them
// contains any of the provided search strings.
func DiagsAny(diags diag.Diagnostics, searchStrings []string) bool {
	if len(diags) == 0 || len(searchStrings) == 0 {
		return false
	}

	for _, d := range diags {
		// Check if current diag contains any of the search strings.
		for _, searchStr := range searchStrings {
			if strings.Contains(d.Detail(), searchStr) {
				return true
			}
		}
	}

	return false
}

// DiagsNone searches through a list of Diagnostics to check if any of them
// contains any of the provided search strings. It returns `true` if none of
// the Diagnostics contain any of the strings.
func DiagsNone(diags diag.Diagnostics, searchStrings []string) bool {
	if len(diags) == 0 || len(searchStrings) == 0 {
		return true
	}

	for _, d := range diags {
		// Check if current diag contains any of the search strings.
		for _, searchStr := range searchStrings {
			if strings.Contains(d.Detail(), searchStr) {
				return false
			}
		}
	}

	return true
}
