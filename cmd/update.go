package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update repo, bottom branches must exist in remote",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			eng, err := newEngine(ctx)
			if err != nil {
				return err
			}

			force, _ := cmd.Flags().GetBool("force")
			branches, err := eng.Update(ctx, force)
			if err != nil {
				return err
			}

			if len(branches) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No bottom branches to update")
				return nil
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Updated bottom branches: %s\n", strings.Join(branches, ", "))
			return nil
		},
	}
	cmd.Flags().BoolP("force", "f", false, "bypass confirmation")
	return cmd
}
