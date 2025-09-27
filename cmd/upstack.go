package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/pmenglund/stacky/internal/engine"
	"github.com/pmenglund/stacky/internal/ui"
)

func newUpstackCmd() *cobra.Command {
	upstackCmd := &cobra.Command{
		Use:     "upstack",
		Aliases: []string{"us"},
		Short:   "Operations on the current upstack",
	}
	upstackCmd.AddCommand(newUpstackInfoCmd())
	upstackCmd.AddCommand(newUpstackPushCmd())
	upstackCmd.AddCommand(newUpstackSyncCmd())
	upstackCmd.AddCommand(newUpstackOntoCmd())
	upstackCmd.AddCommand(newUpstackAsCmd())
	return upstackCmd
}

func newUpstackInfoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "info",
		Aliases: []string{"i"},
		Short:   "Info for current upstack",
		RunE: func(cmd *cobra.Command, args []string) error {
			includePR, _ := cmd.Flags().GetBool("pr")
			if includePR {
				return fmt.Errorf("--pr not implemented in Go rewrite yet")
			}

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

			forest := upstackForestForBranch(branch)
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

func newUpstackPushCmd() *cobra.Command {
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

			return eng.UpstackPush(ctx, engine.PushOptions{
				Remote: remote,
				Force:  force,
			})
		},
	}
	cmd.Flags().BoolP("force", "f", false, "bypass confirmation")
	cmd.Flags().Bool("no-pr", false, "skip creating PRs")
	cmd.Flags().String("remote-name", "origin", "git remote to use")
	return cmd
}

func newUpstackSyncCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Sync",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			eng, err := newEngine(ctx)
			if err != nil {
				return err
			}
			return eng.UpstackSync(ctx)
		},
	}
}

func newUpstackOntoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "onto",
		Aliases: []string{"restack"},
		Short:   "Restack",
		RunE:    notImplemented("upstack onto"),
	}
	cmd.Args = cobra.ExactArgs(1)
	return cmd
}

func newUpstackAsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "as",
		Short: "Upstack branch this as a new stack bottom",
		RunE:  notImplemented("upstack as"),
	}
	cmd.Args = cobra.ExactArgs(1)
	return cmd
}
