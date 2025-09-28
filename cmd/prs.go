package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pmenglund/stacky/internal/githubcli"
	"github.com/pmenglund/stacky/internal/tui"
)

// prsEngine captures the engine capabilities required by the prs command.
type prsEngine interface {
	AuthoredPullRequests(ctx context.Context) ([]githubcli.PullRequest, error)
	ReviewRequestedPullRequests(ctx context.Context) ([]githubcli.PullRequest, error)
	UpdatePRBody(ctx context.Context, number int, body string) error
}

var invokeEditor = defaultEditor

func newPrsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "prs",
		Short: "Interactive PR management - select and edit PR descriptions",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			eng, err := newEngine(ctx)
			if err != nil {
				return err
			}
			return runPrs(cmd, eng)
		},
	}
}

func runPrs(cmd *cobra.Command, eng prsEngine) error {
	ctx := cmd.Context()
	authored, err := eng.AuthoredPullRequests(ctx)
	if err != nil {
		return err
	}
	review, err := eng.ReviewRequestedPullRequests(ctx)
	if err != nil {
		return err
	}

	prs := mergePullRequests(authored, review)
	if len(prs) == 0 {
		cmd.Println("No active pull requests found.")
		return nil
	}

	if !tui.IsTerminal(cmd.InOrStdin()) {
		return fmt.Errorf("interactive PR management requires a terminal")
	}

	for {
		idx, exit, err := selectPullRequest(cmd, prs)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if exit {
			return nil
		}
		if err := editPullRequest(cmd, eng, &prs[idx]); err != nil {
			return err
		}
	}
}

func mergePullRequests(a, b []githubcli.PullRequest) []githubcli.PullRequest {
	if len(a) == 0 && len(b) == 0 {
		return nil
	}
	seen := make(map[string]struct{})
	add := func(dst []githubcli.PullRequest, src []githubcli.PullRequest) []githubcli.PullRequest {
		for _, pr := range src {
			key := pr.ID
			if key == "" {
				key = fmt.Sprintf("%d:%s", pr.Number, pr.HeadRef)
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			dst = append(dst, pr)
		}
		return dst
	}
	merged := make([]githubcli.PullRequest, 0, len(a)+len(b))
	merged = add(merged, a)
	merged = add(merged, b)
	return merged
}

func selectPullRequest(cmd *cobra.Command, prs []githubcli.PullRequest) (index int, exit bool, err error) {
	reader := bufio.NewReader(cmd.InOrStdin())

	cmd.Println()
	cmd.Println("Select a pull request to edit its description:")

	for i, pr := range prs {
		title := pr.Title
		if strings.TrimSpace(title) == "" {
			title = "(no title)"
		}
		cmd.Printf("  %d) #%d %s\n", i+1, pr.Number, title)
	}
	cmd.Println("  0) Exit")

	numberToIndex := make(map[int]int, len(prs))
	for i, pr := range prs {
		if pr.Number > 0 {
			numberToIndex[pr.Number] = i
		}
	}

	for {
		cmd.Print("> ")
		line, readErr := reader.ReadString('\n')
		if readErr != nil {
			return 0, false, readErr
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lowered := strings.ToLower(line)
		if lowered == "q" || lowered == "quit" || lowered == "exit" {
			return 0, true, nil
		}
		if strings.HasPrefix(line, "#") {
			line = strings.TrimSpace(line[1:])
		}
		num, convErr := strconv.Atoi(line)
		if convErr == nil {
			if num == 0 {
				return 0, true, nil
			}
			if num >= 1 && num <= len(prs) {
				return num - 1, false, nil
			}
			if idx, ok := numberToIndex[num]; ok {
				return idx, false, nil
			}
		}
		cmd.Printf("Invalid selection %q. Enter list number, PR number, or 0 to exit.\n", line)
	}
}

func editPullRequest(cmd *cobra.Command, eng prsEngine, pr *githubcli.PullRequest) error {
	cmd.Printf("\nEditing PR #%d - %s\n", pr.Number, pr.Title)
	cmd.Println("Current description:")

	original := pr.Body
	if strings.TrimSpace(original) == "" {
		cmd.Println("(No description)")
		cmd.Println()
	} else {
		cmd.Printf("%s\n\n", original)
	}

	tempFile, err := os.CreateTemp("", "stacky-pr-*.md")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tempName := tempFile.Name()
	if _, err := tempFile.WriteString(original); err != nil {
		tempFile.Close()
		os.Remove(tempName)
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		os.Remove(tempName)
		return fmt.Errorf("close temp file: %w", err)
	}
	defer os.Remove(tempName)

	editor := strings.TrimSpace(os.Getenv("EDITOR"))
	if editor == "" {
		editor = "vim"
	}

	if err := invokeEditor(cmd.Context(), editor, tempName, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
		return fmt.Errorf("launch editor: %w", err)
	}

	contents, err := os.ReadFile(tempName)
	if err != nil {
		return fmt.Errorf("read temp file: %w", err)
	}
	newBody := strings.TrimSpace(string(contents))
	if strings.TrimSpace(original) == newBody {
		cmd.Println("No changes made to PR description.")
		return nil
	}

	cmd.Println("Updating PR description...")
	if err := eng.UpdatePRBody(cmd.Context(), pr.Number, newBody); err != nil {
		return err
	}

	pr.Body = newBody
	cmd.Printf("✓ Successfully updated PR #%d description\n", pr.Number)
	return nil
}

func defaultEditor(ctx context.Context, editor string, path string, stdin io.Reader, stdout, stderr io.Writer) error {
	args := []string{path}
	parts := strings.Fields(editor)
	bin := parts[0]
	if len(parts) > 1 {
		args = append(parts[1:], args...)
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}
