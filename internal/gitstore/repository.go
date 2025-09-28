package gitstore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Runner abstracts command execution, enabling deterministic tests.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) (stdout string, err error)
}

type execRunner struct {
	dir string
}

func (r *execRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = r.dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("gitstore: run %s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}

	return stdout.String(), nil
}

// Repository wraps a git repository root along with a runner used to execute
// git commands. The implementation intentionally shells out to the system git
// binary to avoid bringing in heavy dependencies while parity is validated.
type Repository struct {
	root   string
	runner Runner
}

// Open verifies that path is inside a git worktree and returns a Repository
// rooted at the repository top-level directory.
func Open(ctx context.Context, path string) (*Repository, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("gitstore: resolve path: %w", err)
	}

	runner := &execRunner{dir: abs}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	if _, err := runner.Run(ctx, "git", "rev-parse", "--is-inside-work-tree"); err != nil {
		return nil, fmt.Errorf("gitstore: not a git repository: %w", err)
	}

	out, err := runner.Run(ctx, "git", "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, err
	}

	root := strings.TrimSpace(out)
	return &Repository{root: root, runner: &execRunner{dir: root}}, nil
}

// Init initialises a new git repository at the provided path and returns the
// opened Repository. The directory is created if it does not exist.
func Init(ctx context.Context, path string) (*Repository, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("gitstore: resolve path: %w", err)
	}

	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, fmt.Errorf("gitstore: create directory %s: %w", abs, err)
	}

	runner := &execRunner{dir: abs}
	if _, err := runner.Run(ctx, "git", "init", "-b", "main"); err != nil {
		if _, fallbackErr := runner.Run(ctx, "git", "init"); fallbackErr != nil {
			return nil, fmt.Errorf("gitstore: init repository at %s: %w", abs, err)
		}
	}

	return Open(ctx, abs)
}

// Root returns the absolute repository root.
func (r *Repository) Root() string { return r.root }

// CurrentBranch reports the currently checked-out branch name.
func (r *Repository) CurrentBranch(ctx context.Context) (string, error) {
	out, err := r.runner.Run(ctx, "git", "symbolic-ref", "--short", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// ListBranches returns all local branch names.
func (r *Repository) ListBranches(ctx context.Context) ([]string, error) {
	out, err := r.runner.Run(ctx, "git", "for-each-ref", "--format=%(refname:short)", "refs/heads")
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	branches := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			branches = append(branches, line)
		}
	}
	return branches, nil
}

// ReadRef resolves the provided ref (e.g. "refs/stack-parent/foo") to a commit
// hash. If the ref does not exist, ("", nil) is returned.
func (r *Repository) ReadRef(ctx context.Context, ref string) (string, error) {
	out, err := r.runner.Run(ctx, "git", "rev-parse", ref)
	if err != nil {
		if isMissingRef(err) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// WriteRef updates ref to point at commit. If old is non-empty the update will
// verify the previous value to avoid clobbering concurrent edits.
func (r *Repository) WriteRef(ctx context.Context, ref, commit, old string) error {
	args := []string{"update-ref", ref, commit}
	if old != "" {
		args = append(args, old)
	}
	_, err := r.runner.Run(ctx, "git", args...)
	return err
}

// Checkout switches the working tree to branch.
func (r *Repository) Checkout(ctx context.Context, branch string) error {
	_, err := r.runner.Run(ctx, "git", "checkout", branch)
	return err
}

// RebaseOnto rebases the specified branch onto the given commits using
// `git rebase --onto`. All parameters must be non-empty commit or branch names.
func (r *Repository) RebaseOnto(ctx context.Context, branch, onto, from string) error {
	branch = strings.TrimSpace(branch)
	onto = strings.TrimSpace(onto)
	from = strings.TrimSpace(from)
	if branch == "" || onto == "" || from == "" {
		return fmt.Errorf("gitstore: rebase requires branch, onto, and from refs")
	}
	_, err := r.runner.Run(ctx, "git", "rebase", "--onto", onto, from, branch)
	return err
}

// CreateBranch creates a new branch tracking the current HEAD.
func (r *Repository) CreateBranch(ctx context.Context, branch string) error {
	_, err := r.runner.Run(ctx, "git", "checkout", "-b", branch, "--track")
	return err
}

// Run executes a git command within the repository and returns stdout.
func (r *Repository) Run(ctx context.Context, args ...string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("gitstore: run requires at least one argument")
	}
	return r.runner.Run(ctx, "git", args...)
}

// HeadCommit returns the hash pointed to by HEAD.
func (r *Repository) HeadCommit(ctx context.Context) (string, error) {
	out, err := r.runner.Run(ctx, "git", "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// BranchMergeTarget returns the configured merge target for the branch as
// stored in git config (e.g. refs/heads/main). If the key is unset, an empty
// string and nil error are returned.
func (r *Repository) BranchMergeTarget(ctx context.Context, branch string) (string, error) {
	out, err := r.runner.Run(ctx, "git", "config", fmt.Sprintf("branch.%s.merge", branch))
	if err != nil {
		if isMissingConfig(err) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// Config retrieves a git configuration value, returning an empty string if the
// key is not present.
func (r *Repository) Config(ctx context.Context, key string) (string, error) {
	out, err := r.runner.Run(ctx, "git", "config", key)
	if err != nil {
		if isMissingConfig(err) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func isMissingRef(err error) bool {
	var execErr *exec.ExitError
	if errors.As(err, &execErr) {
		msg := string(execErr.Stderr)
		if strings.Contains(msg, "unknown revision") || strings.Contains(msg, "unknown revision or path") || strings.Contains(msg, "ambiguous argument") {
			return true
		}
	}
	msg := err.Error()
	return strings.Contains(msg, "unknown revision") || strings.Contains(msg, "ambiguous argument")
}

func isMissingConfig(err error) bool {
	var execErr *exec.ExitError
	if errors.As(err, &execErr) {
		msg := string(execErr.Stderr)
		if msg == "" && execErr.ExitCode() == 1 {
			return true
		}
		if strings.Contains(msg, "is not set") || strings.Contains(msg, "No such section") || strings.Contains(msg, "key does not contain a section") {
			return true
		}
	}
	msg := err.Error()
	return strings.Contains(msg, "is not set") || strings.Contains(msg, "does not exist")
}
