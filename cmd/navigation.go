package cmd

import (
	"github.com/spf13/cobra"
)

func newDownCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "down",
		Short: "Go down in the current stack (towards master/main)",
		RunE:  runDown,
	}
}

func newUpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "up",
		Short: "Go up in the current stack (away from master/main)",
		RunE:  runUp,
	}
}

func runDown(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	eng, err := newEngine(ctx)
	if err != nil {
		return err
	}

	current, err := eng.CurrentBranch(ctx)
	if err != nil {
		return err
	}

	target, err := eng.DownBranch(ctx, current)
	if err != nil {
		return err
	}

	if err := eng.Checkout(ctx, target); err != nil {
		return err
	}

	cmd.Printf("Checked out %s\n", target)
	return nil
}

func runUp(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	eng, err := newEngine(ctx)
	if err != nil {
		return err
	}

	current, err := eng.CurrentBranch(ctx)
	if err != nil {
		return err
	}

	target, err := eng.UpBranch(ctx, current)
	if err != nil {
		return err
	}

	if err := eng.Checkout(ctx, target); err != nil {
		return err
	}

	cmd.Printf("Checked out %s\n", target)
	return nil
}
