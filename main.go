package main

import (
	"fmt"
	"os"

	"github.com/pmenglund/stacky/cmd"
)

func main() {
	Execute()
}

// Execute runs the CLI.
func Execute() {
	if err := cmd.RootCmd.Execute(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}
