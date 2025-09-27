package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newPushAliasCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "push",
		Short: "Alias for downstack push",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("stack push not implemented yet")
		},
	}
	cmd.Flags().BoolP("force", "f", false, "bypass confirmation")
	cmd.Flags().Bool("no-pr", false, "skip creating PRs")
	return cmd
}

func newSyncAliasCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Alias for stack sync",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("stack sync not implemented yet")
		},
	}
}

func newRootCheckoutCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "checkout",
		Aliases: []string{"co"},
		Short:   "Checkout a branch",
		RunE:    notImplemented("checkout"),
	}
	cmd.Args = cobra.RangeArgs(0, 1)
	return cmd
}

func newStackOnlyCheckoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sco",
		Short: "Checkout a branch in this stack",
		RunE:  notImplemented("sco"),
	}
}
