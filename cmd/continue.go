package cmd

import (
	"github.com/spf13/cobra"
)

func newContinueCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "continue",
		Short: "Continue previously interrupted command",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			eng, err := newEngine(ctx)
			if err != nil {
				return err
			}

			branch, err := eng.Continue(ctx)
			if err != nil {
				return err
			}

			cmd.Printf("Resumed on branch %s\n", branch)
			return nil
		},
	}
}
