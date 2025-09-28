package gitstore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pmenglund/stacky/internal/config"
)

type stubRunner struct {
	outputs map[string]string
	errors  map[string]error
	calls   [][]string
}

func newStubRunner() *stubRunner {
	return &stubRunner{
		outputs: make(map[string]string),
		errors:  make(map[string]error),
	}
}

func (r *stubRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := append([]string{name}, args...)
	r.calls = append(r.calls, cmd)
	key := strings.Join(cmd, " ")
	if err, ok := r.errors[key]; ok {
		return "", err
	}
	if out, ok := r.outputs[key]; ok {
		return out, nil
	}
	return "", fmt.Errorf("unexpected command: %s", key)
}

func TestSSHMuxEnsureSkipsWhenDisabled(t *testing.T) {
	runner := newStubRunner()
	repo := &Repository{root: t.TempDir(), runner: runner}
	mux := NewSSHMux(repo, config.Config{ShareSSHSession: false})

	require.NoError(t, mux.EnsureMux(context.Background(), "origin"))
	require.Len(t, runner.calls, 0)
}

func TestSSHMuxEnsureSkipsWhenRemoteNotSSH(t *testing.T) {
	ctx := context.Background()
	runner := newStubRunner()
	runner.outputs["git remote -v"] = "origin https://github.com/pmenglund/stacky.git (push)\n"
	repo := &Repository{root: t.TempDir(), runner: runner}
	mux := NewSSHMux(repo, config.Config{ShareSSHSession: true})

	t.Setenv("GIT_SSH_COMMAND", "")

	require.NoError(t, mux.EnsureMux(ctx, "origin"))
	require.Equal(t, "", os.Getenv("GIT_SSH_COMMAND"))
	require.Len(t, runner.calls, 1)
}

func TestSSHMuxEnsureStartsMux(t *testing.T) {
	ctx := context.Background()
	runner := newStubRunner()
	runner.outputs["git remote -v"] = "origin git@github.com:pmenglund/stacky.git (push)\n"
	repo := &Repository{root: t.TempDir(), runner: runner}
	mux := NewSSHMux(repo, config.Config{ShareSSHSession: true})

	t.Setenv("GIT_SSH_COMMAND", "")

	var captured []string
	previous := sshCommandContext
	sshCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		captured = append([]string{name}, args...)
		return exec.CommandContext(ctx, "true")
	}
	defer func() { sshCommandContext = previous }()

	require.NoError(t, mux.EnsureMux(ctx, "origin"))
	require.Equal(t, []string{"ssh", "-o", "ControlMaster=auto", "-o", "ControlPersist=120", "-o", "ControlPath=~/.ssh/stacky-%C", "-MNf", "git@github.com"}, captured)
	require.Equal(t, "ssh -o ControlMaster=auto -o ControlPersist=120 -o ControlPath=~/.ssh/stacky-%C", os.Getenv("GIT_SSH_COMMAND"))
}

func TestSSHMuxCloseMuxRunsExit(t *testing.T) {
	ctx := context.Background()
	runner := newStubRunner()
	runner.outputs["git remote -v"] = "origin git@github.com:pmenglund/stacky.git (push)\n"
	repo := &Repository{root: t.TempDir(), runner: runner}
	mux := NewSSHMux(repo, config.Config{ShareSSHSession: true})

	var captured []string
	previous := sshCommandContext
	sshCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		captured = append([]string{name}, args...)
		return exec.CommandContext(ctx, "true")
	}
	defer func() { sshCommandContext = previous }()

	require.NoError(t, mux.CloseMux(ctx, "origin"))
	require.Equal(t, []string{"ssh", "-o", "ControlMaster=auto", "-o", "ControlPersist=120", "-o", "ControlPath=~/.ssh/stacky-%C", "-O", "exit", "git@github.com"}, captured)
}

func TestSSHMuxEnsurePropagatesRemoteError(t *testing.T) {
	ctx := context.Background()
	runner := newStubRunner()
	runner.errors["git remote -v"] = errors.New("boom")
	repo := &Repository{root: t.TempDir(), runner: runner}
	mux := NewSSHMux(repo, config.Config{ShareSSHSession: true})

	require.Error(t, mux.EnsureMux(ctx, "origin"))
}
