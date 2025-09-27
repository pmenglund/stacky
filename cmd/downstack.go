package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/pmenglund/stacky/internal/ui"
)

func newDownstackCmd() *cobra.Command {
	downstackCmd := &cobra.Command{
		Use:     "downstack",
		Aliases: []string{"ds"},
		Short:   "Operations on the current downstack",
	}
	downstackCmd.AddCommand(newDownstackInfoCmd())
	downstackCmd.AddCommand(newDownstackPushCmd())
	downstackCmd.AddCommand(newDownstackSyncCmd())
	return downstackCmd
}

func newDownstackInfoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "info",
		Aliases: []string{"i"},
		Short:   "Info for current downstack",
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

			forest := downstackForestForBranch(branch)
			var annotations map[string]string
			if includePR {
				names := forestBranchList(forest)
				annotations, err = loadPRAnnotations(ctx, eng, names)
				if err != nil {
					return err
				}
			}
			renderer := ui.New(ui.ColorMode(colorMode))
			out := renderer.RenderForest(forest, ui.ForestRenderOptions{CurrentBranch: currentBranchName, Annotations: annotations})

			fmt.Fprint(cmd.OutOrStdout(), out)
			return nil
		},
	}
	cmd.Flags().Bool("pr", false, "get PR info (slow)")
	return cmd
}

func newDownstackPushCmd() *cobra.Command {
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

			return runPushFlow(cmd, eng, remote, !noPR, force, eng.PlanDownstackPush)
		},
	}
	cmd.Flags().BoolP("force", "f", false, "bypass confirmation")
	cmd.Flags().Bool("no-pr", false, "skip creating PRs")
	cmd.Flags().String("remote-name", "origin", "git remote to use")
	return cmd
}

func newDownstackSyncCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Sync",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			eng, err := newEngine(ctx)
			if err != nil {
				return err
			}
			return eng.DownstackSync(ctx)
		},
	}
}
