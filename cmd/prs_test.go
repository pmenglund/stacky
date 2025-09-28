package cmd

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/pmenglund/stacky/internal/githubcli"
)

type fakePrsEngine struct {
	authored []githubcli.PullRequest
	review   []githubcli.PullRequest
	updates  []struct {
		number int
		body   string
	}
	listErr   error
	updateErr error
}

func (f *fakePrsEngine) AuthoredPullRequests(context.Context) ([]githubcli.PullRequest, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.authored, nil
}

func (f *fakePrsEngine) ReviewRequestedPullRequests(context.Context) ([]githubcli.PullRequest, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.review, nil
}

func (f *fakePrsEngine) UpdatePRBody(_ context.Context, number int, body string) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	f.updates = append(f.updates, struct {
		number int
		body   string
	}{number: number, body: body})
	return nil
}

func TestMergePullRequestsDeduplicatesByID(t *testing.T) {
	pr := githubcli.PullRequest{ID: "abc", Number: 7}
	merged := mergePullRequests([]githubcli.PullRequest{pr}, []githubcli.PullRequest{pr})
	require.Len(t, merged, 1)
	require.Equal(t, 7, merged[0].Number)
}

func TestMergePullRequestsDeduplicatesWithoutID(t *testing.T) {
	pr := githubcli.PullRequest{Number: 8, HeadRef: "feature"}
	merged := mergePullRequests([]githubcli.PullRequest{pr}, []githubcli.PullRequest{{Number: 8, HeadRef: "feature"}})
	require.Len(t, merged, 1)
	require.Equal(t, 8, merged[0].Number)
}

func TestSelectPullRequestSupportsIndex(t *testing.T) {
	cmd := &cobra.Command{}
	in := bytes.NewBufferString("2\n")
	out := &bytes.Buffer{}
	cmd.SetIn(in)
	cmd.SetOut(out)

	prs := []githubcli.PullRequest{{Number: 1, Title: "One"}, {Number: 2, Title: "Two"}}

	idx, exit, err := selectPullRequest(cmd, prs)
	require.NoError(t, err)
	require.False(t, exit)
	require.Equal(t, 1, idx)
	require.Contains(t, out.String(), "#2 Two")
}

func TestSelectPullRequestSupportsPRNumber(t *testing.T) {
	cmd := &cobra.Command{}
	in := bytes.NewBufferString("#42\n")
	out := &bytes.Buffer{}
	cmd.SetIn(in)
	cmd.SetOut(out)

	prs := []githubcli.PullRequest{{Number: 42, Title: "Answer"}}

	idx, exit, err := selectPullRequest(cmd, prs)
	require.NoError(t, err)
	require.False(t, exit)
	require.Equal(t, 0, idx)
}

func TestRunPrsNoPullRequestsShowsMessage(t *testing.T) {
	cmd := &cobra.Command{}
	out := &bytes.Buffer{}
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetOut(out)

	engine := &fakePrsEngine{}

	err := runPrs(cmd, engine)
	require.NoError(t, err)
	require.Contains(t, out.String(), "No active pull requests found.")
}

func TestRunPrsUpdatesPullRequestBody(t *testing.T) {
	cmd := &cobra.Command{}
	in := bytes.NewBufferString("1\n0\n")
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	cmd.SetIn(in)
	cmd.SetOut(out)
	cmd.SetErr(errOut)

	engine := &fakePrsEngine{
		authored: []githubcli.PullRequest{{ID: "1", Number: 5, Title: "Feature", Body: "old"}},
	}

	previous := invokeEditor
	defer func() { invokeEditor = previous }()
	invokeEditor = func(ctx context.Context, editor string, path string, stdin io.Reader, stdout, stderr io.Writer) error {
		return os.WriteFile(path, []byte("new body\n"), 0o600)
	}

	err := runPrs(cmd, engine)
	require.NoError(t, err)

	require.Equal(t, []struct {
		number int
		body   string
	}{{number: 5, body: "new body"}}, engine.updates)
	require.Contains(t, out.String(), "Successfully updated PR #5")
}
