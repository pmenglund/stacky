package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pmenglund/stacky/internal/config"
	"github.com/pmenglund/stacky/internal/engine"
)

type importEngine interface {
	Config() config.Config
	PlanImport(context.Context, string) (engine.ImportPlan, error)
	ExecuteImportPlan(context.Context, engine.ImportPlan) error
}

func newImportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import Graphite stack",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			eng, err := newEngine(ctx)
			if err != nil {
				return err
			}

			force, _ := cmd.Flags().GetBool("force")
			return runImport(cmd, eng, args[0], force)
		},
	}
	cmd.Flags().BoolP("force", "f", false, "bypass confirmation")
	cmd.Args = cobra.ExactArgs(1)
	return cmd
}

func runImport(cmd *cobra.Command, eng importEngine, branch string, force bool) error {
	ctx := cmd.Context()
	plan, err := eng.PlanImport(ctx, branch)
	if err != nil {
		return err
	}

	for _, lookup := range plan.Lookups {
		fmt.Fprintf(cmd.ErrOrStderr(), "Getting PR information for %s\n", lookup)
	}

	fmt.Fprint(cmd.OutOrStdout(), renderImportPlan(plan))

	if len(plan.Actions) == 0 {
		return nil
	}

	if !force && !eng.Config().SkipConfirm {
		if err := confirmProceed(cmd); err != nil {
			return err
		}
	}

	return eng.ExecuteImportPlan(ctx, plan)
}

func renderImportPlan(plan engine.ImportPlan) string {
	if len(plan.Actions) == 0 {
		if plan.TopBranch != "" && plan.BaseBranch != "" && !strings.EqualFold(plan.TopBranch, plan.BaseBranch) {
			return fmt.Sprintf("Branch %s already targets %s; nothing to import\n", plan.TopBranch, plan.BaseBranch)
		}
		return "No branches to import\n"
	}

	var b strings.Builder
	for _, action := range plan.Actions {
		fmt.Fprintf(&b, "- Will set parent of %s to %s at commit %s\n", action.Branch, action.Parent, action.ParentCommit)
	}
	return b.String()
}
