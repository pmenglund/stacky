package cmd

import (
	"github.com/spf13/cobra"
)

func newLandCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "land",
		Short: "Land bottom-most PR on current stack",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			eng, err := newEngine(ctx)
			if err != nil {
				return err
			}

			plan, err := eng.PlanLand(ctx)
			if err != nil {
				return err
			}

			if plan.BranchCount > 1 {
				cmd.Printf("The `land` command only lands the bottom-most branch %s; the current stack has %d branches, ending with %s\n", plan.Branch, plan.BranchCount, plan.CurrentBranch)
			}
			cmd.Printf("- Will land PR #%d (%s) for branch %s into branch %s\n", plan.PRNumber, plan.PRURL, plan.Branch, plan.Parent)

			force, _ := cmd.Flags().GetBool("force")
			if !force && !eng.Config().SkipConfirm {
				if err := confirmProceed(cmd); err != nil {
					return err
				}
			}

			auto, _ := cmd.Flags().GetBool("auto")
			if err := eng.ExecuteLand(ctx, plan, auto); err != nil {
				return err
			}

			cmd.Println()
			cmd.Println("✓ Success! Run `stacky update` to update local state.")
			return nil
		},
	}
	cmd.Flags().BoolP("force", "f", false, "bypass confirmation")
	cmd.Flags().BoolP("auto", "a", false, "automatically merge after checks pass")
	return cmd
}
