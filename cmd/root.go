package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	logLevel   string
	colorMode  string
	remoteName string
)

// NewRootCommand constructs a fully wired Cobra root command for stacky.
func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "stacky",
		Short: "Manage stacks of git branches and pull requests",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if err := validateLogLevel(logLevel); err != nil {
				return err
			}
			if err := validateColorMode(colorMode); err != nil {
				return err
			}
			if remoteName == "" {
				return fmt.Errorf("remote name must not be empty")
			}
			return nil
		},
	}

	root.PersistentFlags().StringVar(&logLevel, "log-level", "info", "set the log level")
	root.PersistentFlags().StringVar(&colorMode, "color", "auto", "colorize output (always|auto|never)")
	root.PersistentFlags().StringVarP(&remoteName, "remote-name", "r", "origin", "git remote used by push commands")

	root.AddCommand(
		newContinueCmd(),
		newDownCmd(),
		newUpCmd(),
		newInfoCmd(),
		newLogCmd(),
		newCommitCmd(),
		newAmendCmd(),
		newBranchCmd(),
		newStackCmd(),
		newUpstackCmd(),
		newDownstackCmd(),
		newUpdateCmd(),
		newImportCmd(),
		newAdoptCmd(),
		newLandCmd(),
		newPushAliasCmd(),
		newSyncAliasCmd(),
		newRootCheckoutCmd(),
		newStackOnlyCheckoutCmd(),
		newInboxCmd(),
		newPrsCmd(),
		newFoldCmd(),
	)

	return root
}

// RootCmd represents the base command invoked without subcommands.
var RootCmd = NewRootCommand()

func validateLogLevel(value string) error {
	switch value {
	case "critical", "error", "warn", "warning", "info", "debug":
		return nil
	default:
		return fmt.Errorf("unsupported log level %q", value)
	}
}

func validateColorMode(value string) error {
	switch value {
	case "always", "auto", "never":
		return nil
	default:
		return fmt.Errorf("unsupported color mode %q", value)
	}
}

func notImplemented(cmdName string) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("%s: not implemented yet", cmdName)
	}
}
