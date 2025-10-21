package errchecker_test

import (
	"errors"
	"fmt"

	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/errchecker"
)

func ExampleContainsAny() {
	// Create some nested errors.
	baseErr := errors.New("database connection failed")
	wrappedErr := fmt.Errorf("query failed: %w", baseErr)
	doubleWrappedErr := fmt.Errorf("operation failed: %w", wrappedErr)

	// Test with multiple search strings.
	searchStrings := []string{"database", "timeout", "network"}
	fmt.Println("Searching for 'database', 'timeout', or 'network':")
	fmt.Printf("Found in nested errors: %v\n", errchecker.ContainsAny(doubleWrappedErr, searchStrings))
	fmt.Printf("ContainsNone should be: %v\n", errchecker.ContainsNone(doubleWrappedErr, searchStrings))

	// Test with non-matching strings.
	fmt.Printf("Found 'xyz': %v\n", errchecker.ContainsAny(doubleWrappedErr, []string{"xyz"}))

	// Example with errors.Join (Go 1.20+)
	err1 := errors.New("network timeout")
	err2 := errors.New("invalid credentials")
	joinedErr := errors.Join(err1, err2)

	fmt.Println("\nSearching in joined errors:")
	fmt.Printf("Found 'timeout': %v\n", errchecker.ContainsAny(joinedErr, []string{"timeout"}))
	fmt.Printf("Found 'credentials': %v\n", errchecker.ContainsAny(joinedErr, []string{"credentials"}))
	fmt.Printf("Found 'database': %v\n", errchecker.ContainsAny(joinedErr, []string{"database"}))

	// Output:
	// Searching for 'database', 'timeout', or 'network':
	// Found in nested errors: true
	// ContainsNone should be: false
	// Found 'xyz': false
	//
	// Searching in joined errors:
	// Found 'timeout': true
	// Found 'credentials': true
	// Found 'database': false
}
