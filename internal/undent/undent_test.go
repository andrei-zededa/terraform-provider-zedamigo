package undent_test

import (
	"fmt"
	"testing"

	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/undent"
)

const errorMsg = "\nexpected %q\ngot %q"

type undentTest struct {
	text, expect string
	repl         []string
}

func TestUndentNoMargin(t *testing.T) {
	texts := []string{
		// No lines indented
		"Hello there.\nHow are you?\nOh good, I'm glad.",
		// Similar with a blank line
		"Hello there.\n\nBoo!",
		// Some lines indented, but overall margin is still zero
		"Hello there.\n  This is indented.",
		// Again, add a blank line.
		"Hello there.\n\n  Boo!\n",
	}

	for _, text := range texts {
		if text != undent.It(text) {
			t.Errorf(errorMsg, text, undent.It(text))
		}
	}
}

func TestUndentEven(t *testing.T) {
	texts := []undentTest{
		{
			// All lines indented by two spaces
			text:   "  Hello there.\n  How are ya?\n  Oh good.",
			expect: "Hello there.\nHow are ya?\nOh good.",
		},
		{
			// Same, with blank lines
			text:   "  Hello there.\n\n  How are ya?\n  Oh good.\n",
			expect: "Hello there.\n\nHow are ya?\nOh good.\n",
		},
		{
			// Now indent one of the blank lines
			text:   "  Hello there.\n  \n  How are ya?\n  Oh good.\n",
			expect: "Hello there.\n\nHow are ya?\nOh good.\n",
		},
		{
			text:   "  Hello there.\n  How are ya?\n  Oh good.\n",
			expect: "Hello there.\nHow are ya?\nOh good.\n",
		},
	}

	for _, text := range texts {
		if text.expect != undent.It(text.text) {
			t.Errorf(errorMsg, text.expect, undent.It(text.text))
		}
	}
}

func TestUndentUneven(t *testing.T) {
	texts := []undentTest{
		{
			// Lines indented unevenly
			text: `
			def foo():
				while 1:
					return foo
			`,
			expect: `
def foo():
	while 1:
		return foo
`,
		},
		{
			// Uneven indentation with a blank line
			text:   "  Foo\n    Bar\n\n   Baz\n",
			expect: "Foo\n  Bar\n\n Baz\n",
		},
		{
			// Uneven indentation with a whitespace-only line
			text:   "  Foo\n    Bar\n \n   Baz\n",
			expect: "Foo\n  Bar\n\n Baz\n",
		},
	}

	for _, text := range texts {
		if text.expect != undent.It(text.text) {
			t.Errorf(errorMsg, text.expect, undent.It(text.text))
		}
	}
}

// undent.It() should not mangle internal tabs.
func TestUndentPreserveInternalTabs(t *testing.T) {
	text := "  hello\tthere\n  how are\tyou?"
	expect := "hello\tthere\nhow are\tyou?"
	if expect != undent.It(text) {
		t.Errorf(errorMsg, expect, undent.It(text))
	}

	// Make sure that it preserves tabs when it's not making any changes at all
	if expect != undent.It(expect) {
		t.Errorf(errorMsg, expect, undent.It(expect))
	}
}

// undent.It() should not mangle tabs in the margin (i.e. tabs and spaces both
// count as margin, but are *not* considered equivalent).
func TestUndentPreserveMarginTabs(t *testing.T) {
	texts := []string{
		"  hello there\n\thow are you?",
		// Same effect even if we have 8 spaces
		"        hello there\n\thow are you?",
	}

	for _, text := range texts {
		d := undent.It(text)
		if text != d {
			t.Errorf(errorMsg, text, d)
		}
	}

	texts2 := []undentTest{
		{
			// undent.It() only removes whitespace that can be uniformly removed!
			text:   "\thello there\n\thow are you?",
			expect: "hello there\nhow are you?",
		},
		{
			text:   "  \thello there\n  \thow are you?",
			expect: "hello there\nhow are you?",
		},
		{
			text:   "  \t  hello there\n  \t  how are you?",
			expect: "hello there\nhow are you?",
		},
		{
			text:   "  \thello there\n  \t  how are you?",
			expect: "hello there\n  how are you?",
		},
	}

	for _, text := range texts2 {
		if text.expect != undent.It(text.text) {
			t.Errorf(errorMsg, text.expect, undent.It(text.text))
		}
	}
}

// undent.It() should do some replacements properly.
func TestUndentWithReplacements(t *testing.T) {
	tests := []undentTest{
		{
			text:   "\thello there\n\thow |are| you?",
			expect: "hello there\nhow `are` you?",
			repl:   []string{"|", "`"},
		},
		{
			text:   "\thello there\n\thow |are| you?",
			expect: "hello there\nhow |are| you?",
			repl:   []string{"|"},
		},
		{
			text:   "\thello there\n\thow |are| you?",
			expect: "hello there\nhow `are` you?",
			repl:   []string{"|", "`", "|"},
		},
		{
			text:   "  \thello there\n  \thow |are| you?",
			expect: "hello there\nhow `are` you?",
			repl:   []string{"|", "`"},
		},
		{
			text:   "  \t  hello there\n  \t  how ''are'' you?",
			expect: "hello there\nhow `are` you?",
			repl:   []string{"''", "`"},
		},
		{
			text:   "  \thello there\n  \t  how ||are|| you?",
			expect: "hello there\n  how `are` you?",
			repl:   []string{"||", "`"},
		},
	}

	for _, tt := range tests {
		if tt.expect != undent.It(tt.text, tt.repl...) {
			t.Errorf(errorMsg, tt.expect, undent.It(tt.text))
		}
	}
}

// undent.Md() is useful.
func TestUndentMarkdown(t *testing.T) {
	tests := []undentTest{
		{
			text:   "\thello there\n\thow |are| you?",
			expect: "hello there\nhow `are` you?",
		},
		{
			text:   "\n\thello there\n\thow |are| you?",
			expect: "hello there\nhow `are` you?",
		},
		{
			text:   "\thello there\n\thow |are| you?",
			expect: "hello there\nhow `are` you?",
		},
		{
			text:   "\thello there\n\thow |are| you?",
			expect: "hello there\nhow `are` you?",
		},
		{
			text:   "  \thello there\n  \thow |are| you?",
			expect: "hello there\nhow `are` you?",
		},
		{
			text:   "  \t  hello there\n  \t  how 'are' you?",
			expect: "hello there\nhow 'are' you?",
		},
		{
			text:   "  \thello there\n  \t  how ||are|| you?",
			expect: "hello there\n  how ``are`` you?",
		},
	}

	for _, tt := range tests {
		if tt.expect != undent.Md(tt.text) {
			t.Errorf(errorMsg, tt.expect, undent.It(tt.text))
		}
	}
}

func Example_undent_It() {
	s := `
		Lorem ipsum dolor sit amet,
		consectetur adipiscing elit.
		Curabitur justo tellus, facilisis nec efficitur dictum,
		fermentum vitae ligula. Sed eu convallis sapien.`
	fmt.Println(undent.It(s))
	fmt.Println("-------------")
	fmt.Println(s)
	// Output:
	// Lorem ipsum dolor sit amet,
	// consectetur adipiscing elit.
	// Curabitur justo tellus, facilisis nec efficitur dictum,
	// fermentum vitae ligula. Sed eu convallis sapien.
	// -------------
	//
	//		Lorem ipsum dolor sit amet,
	//		consectetur adipiscing elit.
	//		Curabitur justo tellus, facilisis nec efficitur dictum,
	//		fermentum vitae ligula. Sed eu convallis sapien.
}

func BenchmarkUndent(b *testing.B) {
	for b.Loop() {
		undent.It(`Lorem ipsum dolor sit amet, consectetur adipiscing elit.
		Curabitur justo tellus, facilisis nec efficitur dictum,
		fermentum vitae ligula. Sed eu convallis sapien.`)
	}
}
