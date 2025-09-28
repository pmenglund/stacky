package cmd

import (
	"bufio"
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pmenglund/stacky/internal/config"
	"github.com/pmenglund/stacky/internal/engine"
	"github.com/pmenglund/stacky/internal/tui"
)

type pushPlanBuilder func(context.Context, string, bool) (engine.PushPlan, error)

type pushExecutor interface {
	Config() config.Config
	ExecutePushPlan(context.Context, engine.PushPlan) error
}

func runPushFlow(cmd *cobra.Command, eng pushExecutor, remote string, includePR bool, force bool, builder pushPlanBuilder) error {
	ctx := cmd.Context()
	plan, err := builder(ctx, remote, includePR)
	if err != nil {
		return err
	}

	fmt.Fprint(cmd.OutOrStdout(), renderPushPlan(plan))

	if !planRequiresWork(plan) {
		return nil
	}

	if !force && !eng.Config().SkipConfirm {
		if err := confirmProceed(cmd); err != nil {
			return err
		}
	}

	return eng.ExecutePushPlan(ctx, plan)
}

func planRequiresWork(plan engine.PushPlan) bool {
	for _, action := range plan.Actions {
		if action.IsBase {
			continue
		}
		if action.Push || action.PRAction != engine.PRActionNone {
			return true
		}
	}
	return false
}

func renderPushPlan(plan engine.PushPlan) string {
	var b strings.Builder
	if len(plan.Actions) == 0 {
		b.WriteString("No branches to push\n")
		return b.String()
	}

	for _, action := range plan.Actions {
		if action.IsBase {
			fmt.Fprintf(&b, "✓ Not pushing base branch %s\n", action.Branch)
			continue
		}

		if action.Push {
			fmt.Fprintf(&b, "- Will push branch %s to %s/%s\n", action.Branch, plan.Remote, action.Branch)
		} else if action.RemoteExists {
			fmt.Fprintf(&b, "✓ Not pushing branch %s, synced with remote %s/%s\n", action.Branch, plan.Remote, action.Branch)
		} else {
			fmt.Fprintf(&b, "✓ Not pushing branch %s\n", action.Branch)
		}

		switch action.PRAction {
		case engine.PRActionCreate:
			fmt.Fprintf(&b, "- Will create PR for branch %s\n", action.Branch)
		case engine.PRActionUpdateBase:
			base := action.PRBase
			if base == "" {
				base = "current base"
			}
			fmt.Fprintf(&b, "- Branch %s already has open PR #%d; will change PR base from %s to %s\n", action.Branch, action.PRNumber, base, action.Parent)
		default:
			if action.HasPR && action.PRNumber != 0 {
				fmt.Fprintf(&b, "✓ Branch %s already has open PR #%d\n", action.Branch, action.PRNumber)
			}
		}
	}

	return b.String()
}

func confirmProceed(cmd *cobra.Command) error {
	in := cmd.InOrStdin()
	if !tui.IsTerminal(in) {
		return fmt.Errorf("standard input is not a terminal; use --force to skip confirmation")
	}

	reader := bufio.NewReader(in)
	out := cmd.OutOrStdout()
	fmt.Fprintln(out)

	for {
		fmt.Fprint(out, "Proceed? [yes/no] ")
		line, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		answer := strings.ToLower(strings.TrimSpace(line))
		switch answer {
		case "y", "yes":
			return nil
		case "n", "no":
			return fmt.Errorf("not confirmed")
		default:
			fmt.Fprintln(out, "Please answer yes or no")
		}
	}
}
