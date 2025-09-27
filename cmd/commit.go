package cmd

import "github.com/spf13/cobra"

func newCommitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "commit",
		Short: "Commit",
		RunE:  notImplemented("commit"),
	}
	cmd.Flags().StringP("message", "m", "", "commit message")
	cmd.Flags().Bool("amend", false, "amend last commit")
	cmd.Flags().Bool("allow-empty", false, "allow empty commit")
	cmd.Flags().Bool("no-edit", false, "skip editor")
	cmd.Flags().BoolP("add-all", "a", false, "add all files to commit")
	cmd.Flags().Bool("no-verify", false, "bypass pre-commit and commit-msg hooks")
	return cmd
}

func newAmendCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "amend",
		Short: "Shortcut for amending last commit",
		RunE:  notImplemented("amend"),
	}
	cmd.Flags().Bool("no-verify", false, "bypass pre-commit and commit-msg hooks")
	return cmd
}
