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
			ctx := cmd.Context()
			eng, err := newEngine(ctx)
			if err != nil {
				return err
			}

			force, _ := cmd.Flags().GetBool("force")
			noPR, _ := cmd.Flags().GetBool("no-pr")

			return runPushFlow(cmd, eng, remoteName, !noPR, force, eng.PlanDownstackPush)
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
			ctx := cmd.Context()
			eng, err := newEngine(ctx)
			if err != nil {
				return err
			}
			return eng.StackSync(ctx)
		},
	}
}

func newRootCheckoutCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "checkout",
		Aliases: []string{"co"},
		Short:   "Checkout a branch",
		Args:    cobra.RangeArgs(0, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			eng, err := newEngine(ctx)
			if err != nil {
				return err
			}

			if len(args) == 1 {
				return eng.Checkout(ctx, args[0])
			}

			forest, err := eng.StackForest(ctx)
			if err != nil {
				return err
			}
			current, err := eng.CurrentBranch(ctx)
			if err != nil {
				return err
			}

			allowed := forestBranchNames(forest)
			options := make([]string, 0, len(allowed))
			for name := range allowed {
				if name == current {
					continue
				}
				options = append(options, name)
			}

			if len(options) == 0 {
				return fmt.Errorf("no other branches available to checkout")
			}

			selected, err := selectBranch(cmd, options, "Select branch", current)
			if err != nil {
				return err
			}

			return eng.Checkout(ctx, selected)
		},
	}
	return cmd
}

func newStackOnlyCheckoutCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sco",
		Short: "Checkout a branch in this stack",
		Args:  cobra.RangeArgs(0, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			delegate := newStackCheckoutCmd()
			delegate.SetContext(cmd.Context())
			delegate.SetIn(cmd.InOrStdin())
			delegate.SetOut(cmd.OutOrStdout())
			delegate.SetErr(cmd.ErrOrStderr())
			if delegate.Args != nil {
				if err := delegate.Args(delegate, args); err != nil {
					return err
				}
			}
			return delegate.RunE(delegate, args)
		},
	}
	return cmd
}
