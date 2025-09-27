package cmd

import "github.com/spf13/cobra"

func newAdoptCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "adopt",
		Short: "Adopt one branch",
		RunE:  notImplemented("adopt"),
	}
	cmd.Args = cobra.ExactArgs(1)
	return cmd
}
