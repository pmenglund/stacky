package tui

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// SelectOptions controls how Select renders the prompt and options.
type SelectOptions struct {
	// Prompt is printed before the list of options. When empty, no prompt is
	// shown.
	Prompt string
	// Highlight marks an option (typically the current value) with a " *" suffix.
	Highlight string
}

// Select presents the provided options to the user and returns the chosen value.
//
// The function accepts numeric selections (1-based index) or exact option text.
// If the reader reaches EOF before a valid selection is made, the EOF error is
// returned to the caller.
func Select(in io.Reader, out io.Writer, options []string, opts SelectOptions) (string, error) {
	if len(options) == 0 {
		return "", fmt.Errorf("no options available")
	}

	if opts.Prompt != "" {
		fmt.Fprintf(out, "%s:\n", opts.Prompt)
	}

	for i, option := range options {
		marker := ""
		if option == opts.Highlight {
			marker = " *"
		}
		fmt.Fprintf(out, "  %d) %s%s\n", i+1, option, marker)
	}

	reader := bufio.NewReader(in)
	for {
		fmt.Fprint(out, "> ")
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if index, err := strconv.Atoi(line); err == nil {
			if index >= 1 && index <= len(options) {
				return options[index-1], nil
			}
		}

		for _, option := range options {
			if option == line {
				return option, nil
			}
		}

		fmt.Fprintf(out, "Invalid selection %q. Enter number or name from list.\n", line)
	}
}

// IsTerminal reports whether the supplied reader is backed by a terminal.
//
// Non-*os.File readers are treated as terminals to keep tests and injected
// readers working without additional plumbing.
func IsTerminal(reader io.Reader) bool {
	file, ok := reader.(*os.File)
	if !ok {
		return true
	}

	info, err := file.Stat()
	if err != nil {
		return true
	}

	return (info.Mode() & os.ModeCharDevice) != 0
}
