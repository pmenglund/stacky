package cmd

import "github.com/spf13/cobra"

func newLandCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "land",
		Short: "Land bottom-most PR on current stack",
		RunE:  notImplemented("land"),
	}
	cmd.Flags().BoolP("force", "f", false, "bypass confirmation")
	cmd.Flags().BoolP("auto", "a", false, "automatically merge after checks pass")
	return cmd
}
