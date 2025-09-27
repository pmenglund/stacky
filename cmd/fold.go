package cmd

import "github.com/spf13/cobra"

func newFoldCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fold",
		Short: "Fold current branch into parent branch and delete current branch",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			eng, err := newEngine(ctx)
			if err != nil {
				return err
			}

			allowEmpty, _ := cmd.Flags().GetBool("allow-empty")
			return eng.Fold(ctx, allowEmpty)
		},
	}
	cmd.Flags().Bool("allow-empty", false, "allow empty commits during cherry-pick")
	return cmd
}
