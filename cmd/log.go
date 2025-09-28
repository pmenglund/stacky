package cmd

import (
	"github.com/spf13/cobra"
)

func newLogCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "log",
		Short: "Show git log with conditional merge handling",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			eng, err := newEngine(ctx)
			if err != nil {
				return err
			}

			out, err := eng.Log(ctx)
			if err != nil {
				return err
			}

			cmd.Print(out)
			return nil
		},
	}
}
