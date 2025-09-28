package cmd

import (
	"github.com/spf13/cobra"

	"github.com/pmenglund/stacky/internal/ui"
)

func newInfoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "info",
		Short: "Stack info",
		RunE: func(cmd *cobra.Command, args []string) error {
			includePR, _ := cmd.Flags().GetBool("pr")

			ctx := cmd.Context()

			eng, err := newEngine(ctx)
			if err != nil {
				return err
			}

			currentBranch, err := eng.CurrentBranch(ctx)
			if err != nil {
				return err
			}

			forest, err := eng.StackForest(ctx)
			if err != nil {
				return err
			}

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
			out := renderer.RenderForest(forest, ui.ForestRenderOptions{CurrentBranch: currentBranch, Annotations: annotations})

			cmd.Print(out)
			return nil
		},
	}
	cmd.Flags().Bool("pr", false, "get PR info (slow)")
	return cmd
}
