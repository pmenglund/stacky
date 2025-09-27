package cmd

import "github.com/spf13/cobra"

func newImportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import Graphite stack",
		RunE:  notImplemented("import"),
	}
	cmd.Flags().BoolP("force", "f", false, "bypass confirmation")
	cmd.Args = cobra.ExactArgs(1)
	return cmd
}
