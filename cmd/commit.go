package cmd

import (
	"github.com/spf13/cobra"

	"github.com/pmenglund/stacky/internal/engine"
)

func newCommitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "commit",
		Short: "Create a commit on the current stack branch",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts, err := commitOptionsFromFlags(cmd)
			if err != nil {
				return err
			}
			return runCommit(cmd, opts)
		},
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
		Short: "Shortcut for amending the last commit without opening an editor",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			noVerify, err := cmd.Flags().GetBool("no-verify")
			if err != nil {
				return err
			}
			return runCommit(cmd, engine.CommitOptions{Amend: true, NoEdit: true, NoVerify: noVerify})
		},
	}
	cmd.Flags().Bool("no-verify", false, "bypass pre-commit and commit-msg hooks")
	return cmd
}

func commitOptionsFromFlags(cmd *cobra.Command) (engine.CommitOptions, error) {
	message, err := cmd.Flags().GetString("message")
	if err != nil {
		return engine.CommitOptions{}, err
	}
	amend, err := cmd.Flags().GetBool("amend")
	if err != nil {
		return engine.CommitOptions{}, err
	}
	allowEmpty, err := cmd.Flags().GetBool("allow-empty")
	if err != nil {
		return engine.CommitOptions{}, err
	}
	noEdit, err := cmd.Flags().GetBool("no-edit")
	if err != nil {
		return engine.CommitOptions{}, err
	}
	addAll, err := cmd.Flags().GetBool("add-all")
	if err != nil {
		return engine.CommitOptions{}, err
	}
	noVerify, err := cmd.Flags().GetBool("no-verify")
	if err != nil {
		return engine.CommitOptions{}, err
	}

	return engine.CommitOptions{
		Message:    message,
		Amend:      amend,
		AllowEmpty: allowEmpty,
		NoEdit:     noEdit,
		AddAll:     addAll,
		NoVerify:   noVerify,
	}, nil
}

func runCommit(cmd *cobra.Command, opts engine.CommitOptions) error {
	ctx := cmd.Context()
	eng, err := newEngine(ctx)
	if err != nil {
		return err
	}
	return eng.Commit(ctx, opts)
}
