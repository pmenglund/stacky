package cmd

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"github.com/pmenglund/stacky/internal/tui"
	"github.com/pmenglund/stacky/internal/ui"
)

func newStackCmd() *cobra.Command {
	stackCmd := &cobra.Command{
		Use:     "stack",
		Aliases: []string{"s"},
		Short:   "Operations on the full current stack",
	}
	stackCmd.AddCommand(newStackInfoCmd())
	stackCmd.AddCommand(newStackPushCmd())
	stackCmd.AddCommand(newStackSyncCmd())
	stackCmd.AddCommand(newStackCheckoutCmd())
	return stackCmd
}

func newStackInfoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "info",
		Aliases: []string{"i"},
		Short:   "Info for current stack",
		RunE: func(cmd *cobra.Command, args []string) error {
			includePR, _ := cmd.Flags().GetBool("pr")

			ctx := cmd.Context()

			eng, err := newEngine(ctx)
			if err != nil {
				return err
			}

			currentBranchName, err := eng.CurrentBranch(ctx)
			if err != nil {
				return err
			}

			branch, err := eng.Branch(ctx, currentBranchName)
			if err != nil {
				return err
			}

			forest := stackForestForBranch(branch)
			var annotations map[string]string
			if includePR {
				names := forestBranchList(forest)
				annotations, err = loadPRAnnotations(ctx, eng, names)
				if err != nil {
					return err
				}
			}
			mode := ui.ResolveColorMode(ui.ColorMode(colorMode), cmd.OutOrStdout())
			renderer := ui.New(mode)
			out := renderer.RenderForest(forest, ui.ForestRenderOptions{CurrentBranch: currentBranchName, Annotations: annotations})

			cmd.Print(out)
			return nil
		},
	}
	cmd.Flags().Bool("pr", false, "get PR info (slow)")
	return cmd
}

func newStackPushCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "push",
		Short: "Push",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			eng, err := newEngine(ctx)
			if err != nil {
				return err
			}

			remote, _ := cmd.Flags().GetString("remote-name")
			force, _ := cmd.Flags().GetBool("force")
			noPR, _ := cmd.Flags().GetBool("no-pr")

			return runPushFlow(cmd, eng, remote, !noPR, force, eng.PlanStackPush)
		},
	}
	cmd.Flags().BoolP("force", "f", false, "bypass confirmation")
	cmd.Flags().Bool("no-pr", false, "skip creating PRs")
	cmd.Flags().String("remote-name", "origin", "git remote to use")
	return cmd
}

func newStackSyncCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Sync",
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

func newStackCheckoutCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "checkout",
		Aliases: []string{"co"},
		Short:   "Checkout a branch in this stack",
		Args:    cobra.RangeArgs(0, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			eng, err := newEngine(ctx)
			if err != nil {
				return err
			}

			currentBranchName, err := eng.CurrentBranch(ctx)
			if err != nil {
				return err
			}

			branch, err := eng.Branch(ctx, currentBranchName)
			if err != nil {
				return err
			}

			forest := stackForestForBranch(branch)
			allowed := forestBranchNames(forest)
			target := ""
			if len(args) == 0 {
				options := make([]string, 0, len(allowed))
				for name := range allowed {
					if name == currentBranchName {
						continue
					}
					options = append(options, name)
				}
				if len(options) == 0 {
					return fmt.Errorf("no other branches available to checkout")
				}
				sort.Strings(options)
				selected, err := selectBranch(cmd, options, "Select branch in stack", currentBranchName)
				if err != nil {
					return err
				}
				target = selected
			} else {
				target = args[0]
			}

			if _, ok := allowed[target]; !ok {
				return fmt.Errorf("branch %s is not in the current stack", target)
			}

			return eng.Checkout(ctx, target)
		},
	}
	return cmd
}

func selectBranch(cmd *cobra.Command, options []string, prompt, current string) (string, error) {
	if len(options) == 0 {
		return "", fmt.Errorf("no branches available")
	}
	if len(options) == 1 {
		return options[0], nil
	}

	in := cmd.InOrStdin()
	if !tui.IsTerminal(in) {
		return "", fmt.Errorf("interactive branch selection requires a terminal; pass the branch name explicitly")
	}

	sorted := append([]string(nil), options...)
	sort.Strings(sorted)

	selection, err := tui.Select(in, cmd.OutOrStdout(), sorted, tui.SelectOptions{Prompt: prompt, Highlight: current})
	if err != nil {
		return "", err
	}
	return selection, nil
}
