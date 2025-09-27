package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/pmenglund/stacky/internal/engine"
)

func newBranchCmd() *cobra.Command {
	branchCmd := &cobra.Command{
		Use:     "branch",
		Aliases: []string{"b"},
		Short:   "Operations on branches",
	}

	branchCmd.AddCommand(newBranchUpCmd())
	branchCmd.AddCommand(newBranchDownCmd())
	branchCmd.AddCommand(newBranchNewCmd())
	branchCmd.AddCommand(newBranchCommitCmd())
	branchCmd.AddCommand(newBranchCheckoutCmd())

	return branchCmd
}

func newBranchUpCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "up",
		Aliases: []string{"u"},
		Short:   "Move upstack",
		RunE:    runUp,
	}
}

func newBranchDownCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "down",
		Aliases: []string{"d"},
		Short:   "Move downstack",
		RunE:    runDown,
	}
}

func newBranchNewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "new",
		Aliases: []string{"create"},
		Short:   "Create a new branch",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			eng, err := newEngine(ctx)
			if err != nil {
				return err
			}
			return eng.CreateBranch(ctx, args[0])
		},
	}
	cmd.Args = cobra.ExactArgs(1)
	return cmd
}

func newBranchCommitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "commit",
		Short: "Create a new branch and commit all changes",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			eng, err := newEngine(ctx)
			if err != nil {
				return err
			}

			name := args[0]
			if err := eng.CreateBranch(ctx, name); err != nil {
				return err
			}

			addAll, _ := cmd.Flags().GetBool("add-all")
			noVerify, _ := cmd.Flags().GetBool("no-verify")
			message, _ := cmd.Flags().GetString("message")
			allowEmpty, _ := cmd.Flags().GetBool("allow-empty")
			amend, _ := cmd.Flags().GetBool("amend")
			noEdit, _ := cmd.Flags().GetBool("no-edit")

			return eng.Commit(ctx, engine.CommitOptions{
				AddAll:     addAll,
				NoVerify:   noVerify,
				AllowEmpty: allowEmpty,
				Amend:      amend,
				NoEdit:     noEdit,
				Message:    message,
			})
		},
	}
	cmd.Args = cobra.ExactArgs(1)
	cmd.Flags().StringP("message", "m", "", "commit message")
	cmd.Flags().BoolP("add-all", "a", false, "add all files to commit")
	cmd.Flags().Bool("no-verify", false, "bypass pre-commit and commit-msg hooks")
	cmd.Flags().Bool("allow-empty", false, "allow empty commit")
	cmd.Flags().Bool("amend", false, "amend the previous commit")
	cmd.Flags().Bool("no-edit", false, "reuse existing commit message when amending")
	return cmd
}

func newBranchCheckoutCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "checkout",
		Aliases: []string{"co"},
		Short:   "Checkout a branch",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			eng, err := newEngine(ctx)
			if err != nil {
				return err
			}

			current, err := eng.CurrentBranch(ctx)
			if err != nil {
				return err
			}

			var target string
			if len(args) > 0 {
				target = args[0]
			} else {
				forest, err := eng.StackForest(ctx)
				if err != nil {
					return err
				}
				options := filterOutBranch(forestBranchList(forest), current)
				if len(options) == 0 {
					return fmt.Errorf("no other branches available to checkout")
				}
				target, err = selectBranch(cmd, options, "Select branch", current)
				if err != nil {
					return err
				}
			}

			if _, err := eng.Branch(ctx, target); err != nil {
				return err
			}

			return eng.Checkout(ctx, target)
		},
	}
	cmd.Args = cobra.RangeArgs(0, 1)
	return cmd
}
