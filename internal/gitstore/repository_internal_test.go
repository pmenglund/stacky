package gitstore

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type recordingRunner struct {
	name string
	args []string
	err  error
}

func (r *recordingRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	r.name = name
	r.args = append([]string(nil), args...)
	return "", r.err
}

func TestRepositoryRebaseOntoBuildsCommand(t *testing.T) {
	runner := &recordingRunner{}
	repo := &Repository{root: t.TempDir(), runner: runner}

	err := repo.RebaseOnto(context.Background(), "feature", "onto", "from")
	require.NoError(t, err)
	require.Equal(t, "git", runner.name)
	require.Equal(t, []string{"rebase", "--onto", "onto", "from", "feature"}, runner.args)
}

func TestRepositoryRebaseOntoValidatesInputs(t *testing.T) {
	repo := &Repository{root: t.TempDir(), runner: &recordingRunner{}}

	cases := []struct {
		name   string
		branch string
		onto   string
		from   string
	}{
		{"empty branch", "", "onto", "from"},
		{"empty onto", "feature", "", "from"},
		{"empty from", "feature", "onto", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := repo.RebaseOnto(context.Background(), tc.branch, tc.onto, tc.from)
			require.Error(t, err)
		})
	}
}

func TestRepositoryRebaseOntoPropagatesErrors(t *testing.T) {
	runner := &recordingRunner{err: errors.New("boom")}
	repo := &Repository{root: t.TempDir(), runner: runner}

	err := repo.RebaseOnto(context.Background(), "feature", "onto", "from")
	require.Error(t, err)
}
