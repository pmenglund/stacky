package githubcli_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pmenglund/stacky/internal/githubcli"
)

type fakeRunner struct {
	out  string
	err  error
	args []string
}

func (f *fakeRunner) Run(_ context.Context, args ...string) (string, error) {
	f.args = append(f.args, args...)
	return f.out, f.err
}

func TestListPRsParsesOutput(t *testing.T) {
	runner := &fakeRunner{out: `[{"id":"abc","number":1,"state":"OPEN","url":"https://example.com","title":"Title","baseRefName":"main","headRefName":"feature","mergeable":"MERGEABLE","updatedAt":"2024-04-07T00:00:00Z","createdAt":"2024-04-01T00:00:00Z","reviewDecision":"APPROVED","isDraft":false,"author":{"login":"octo"},"reviewRequests":[{"login":"reviewer"}] }]`}
	client := githubcli.NewWithRunner(runner)

	prs, err := client.ListPRs(context.Background(), githubcli.ListParams{Author: "@me", State: "open"})
	require.NoError(t, err)
	require.Equal(t, 1, len(prs))
	require.Equal(t, "feature", prs[0].HeadRef)
	require.Equal(t, []string{"pr", "list", "--json", "id,number,state,url,title,baseRefName,headRefName,mergeable,updatedAt,createdAt,reviewDecision,isDraft,author.login,reviewRequests[].login", "--state", "open", "--author", "@me"}, runner.args)
}

func TestListPRsPropagatesError(t *testing.T) {
	runner := &fakeRunner{err: errors.New("boom")}
	client := githubcli.NewWithRunner(runner)

	_, err := client.ListPRs(context.Background(), githubcli.ListParams{})
	require.Error(t, err)
}

func TestListPRsWithSearchAddsFlag(t *testing.T) {
	runner := &fakeRunner{out: `[]`}
	client := githubcli.NewWithRunner(runner)

	_, err := client.ListPRs(context.Background(), githubcli.ListParams{Search: "review-requested:@me"})
	require.NoError(t, err)
	require.Contains(t, runner.args, "review-requested:@me")
}
