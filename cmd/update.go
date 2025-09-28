package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pmenglund/stacky/internal/config"
	"github.com/pmenglund/stacky/internal/engine"
)

type updateEngine interface {
	Config() config.Config
	PlanUpdate(context.Context, string) (engine.UpdatePlan, error)
	ExecuteUpdatePlan(context.Context, engine.UpdatePlan) error
}

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
			return runUpdateFlow(cmd, eng, remoteName, force)
		},
	}
	cmd.Flags().BoolP("force", "f", false, "bypass confirmation")
	return cmd
}

func runUpdateFlow(cmd *cobra.Command, eng updateEngine, remote string, force bool) error {
	ctx := cmd.Context()
	plan, err := eng.PlanUpdate(ctx, remote)
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	fmt.Fprint(out, renderUpdatePlan(plan))

	if !updatePlanHasWork(plan) {
		return nil
	}

	if len(plan.Deletions) > 0 && !force && !eng.Config().SkipConfirm {
		if err := confirmProceed(cmd); err != nil {
			return err
		}
	}

	return eng.ExecuteUpdatePlan(ctx, plan)
}

func renderUpdatePlan(plan engine.UpdatePlan) string {
	var b strings.Builder

	if len(plan.Bottoms) == 0 {
		b.WriteString("No bottom branches tracked.\n")
	} else {
		for _, bottom := range plan.Bottoms {
			if bottom.NeedsUpdate {
				fmt.Fprintf(&b, "- Will fast-forward bottom branch %s to %s/%s\n", bottom.Branch, plan.Remote, bottom.Branch)
			} else {
				fmt.Fprintf(&b, "✓ Bottom branch %s already matches %s/%s\n", bottom.Branch, plan.Remote, bottom.Branch)
			}
		}
	}

	if len(plan.Deletions) == 0 {
		if !updatePlanHasWork(plan) {
			b.WriteString("No changes required.\n")
		}
	} else {
		for _, deletion := range plan.Deletions {
			fmt.Fprintf(&b, "- Will delete branch %s; PR #%d merged into %s\n", deletion.Branch, deletion.PRNumber, deletion.Parent)
			for _, child := range deletion.Children {
				fmt.Fprintf(&b, "- Will reparent branch %s onto %s\n", child, deletion.Parent)
			}
		}
	}

	return b.String()
}

func updatePlanHasWork(plan engine.UpdatePlan) bool {
	if len(plan.Deletions) > 0 {
		return true
	}
	for _, bottom := range plan.Bottoms {
		if bottom.NeedsUpdate {
			return true
		}
	}
	return false
}
