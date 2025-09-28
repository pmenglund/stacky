package ui

import (
	"io"
	"os"

	"golang.org/x/term"
)

type fdWriter interface {
	Fd() uintptr
}

// ResolveColorMode converts a requested color mode into the actual mode that
// should be used for the provided writer. When "auto" is requested we only
// enable ANSI styling if the writer is a terminal and NO_COLOR is not set.
func ResolveColorMode(requested ColorMode, writer io.Writer) ColorMode {
	switch requested {
	case ColorModeAlways:
		return ColorModeAlways
	case ColorModeNever:
		return ColorModeNever
	}

	if _, disable := os.LookupEnv("NO_COLOR"); disable {
		return ColorModeNever
	}

	if f, ok := writer.(fdWriter); ok {
		if term.IsTerminal(int(f.Fd())) {
			return ColorModeAlways
		}
	}

	return ColorModeNever
}
