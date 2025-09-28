package githubcli_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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
	runner := &fakeRunner{out: `[{"id":"abc","number":1,"state":"OPEN","url":"https://example.com","title":"Title","baseRefName":"main","headRefName":"feature","mergeable":"MERGEABLE","mergeStateStatus":"CLEAN","updatedAt":"2024-04-07T00:00:00Z","createdAt":"2024-04-01T00:00:00Z","reviewDecision":"APPROVED","isDraft":false,"author":{"login":"octo"},"reviewRequests":[{"login":"reviewer"}],"statusCheckRollup":[{"state":"SUCCESS"}],"body":"body"}]`}
	client := githubcli.NewWithRunner(runner)

	prs, err := client.ListPRs(context.Background(), githubcli.ListParams{Author: "@me", State: "open"})
	require.NoError(t, err)
	require.Equal(t, 1, len(prs))
	require.Equal(t, "feature", prs[0].HeadRef)
	require.Equal(t, []string{"pr", "list", "--json", "id,number,state,url,title,baseRefName,headRefName,mergeable,mergeStateStatus,updatedAt,createdAt,reviewDecision,isDraft,author.login,reviewRequests[].login,statusCheckRollup,body", "--state", "open", "--author", "@me"}, runner.args)
}

func TestListPRsIncludesExtraFields(t *testing.T) {
	runner := &fakeRunner{out: `[{"id":"def","number":2,"state":"OPEN","url":"https://example.com/pr/2","title":"Second","baseRefName":"main","headRefName":"feature2","mergeable":"MERGEABLE","mergeStateStatus":"CLEAN","updatedAt":"2024-04-08T00:00:00Z","createdAt":"2024-04-02T00:00:00Z","reviewDecision":"REVIEW_REQUIRED","isDraft":false,"author":{"login":"octo"},"reviewRequests":[],"statusCheckRollup":[],"body":"body","commits":[{"oid":"deadbeef"}]}]`}
	client := githubcli.NewWithRunner(runner)

	prs, err := client.ListPRs(context.Background(), githubcli.ListParams{Head: "feature2", Fields: []string{"commits", "headRefName"}})
	require.NoError(t, err)
	require.Len(t, prs, 1)
	require.Equal(t, "deadbeef", prs[0].Commits[0].OID)
	require.GreaterOrEqual(t, len(runner.args), 4)
	require.Equal(t, "pr", runner.args[0])
	require.Equal(t, "list", runner.args[1])
	require.Equal(t, "--json", runner.args[2])
	jsonArgs := runner.args[3]
	require.True(t, strings.Contains(jsonArgs, "commits"))
	require.False(t, strings.Contains(jsonArgs, "headRefName,headRefName"))
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

func TestGetPRForBranchParsesFixture(t *testing.T) {
	fixturePath := filepath.Join("testdata", "branch_prs.json")
	data, err := os.ReadFile(fixturePath)
	require.NoError(t, err)

	runner := &fakeRunner{out: string(data)}
	client := githubcli.NewWithRunner(runner)

	prs, err := client.GetPRForBranch(context.Background(), githubcli.GetPRParams{Branch: "feature", Fields: []string{"commits"}})
	require.NoError(t, err)
	require.NotNil(t, prs.Open)
	require.Equal(t, "PR_kwABC123", prs.Open.ID)
	require.Equal(t, 1, len(prs.All))
	require.Equal(t, "feature", prs.Open.HeadRef)
	expectedJSON := "id,number,state,url,title,baseRefName,headRefName,mergeable,mergeStateStatus,updatedAt,createdAt,reviewDecision,isDraft,author.login,reviewRequests[].login,statusCheckRollup,body,commits"
	require.Equal(t, []string{"pr", "list", "--json", expectedJSON, "--state", "all", "--head", "feature"}, runner.args)
}

func TestGetPRForBranchErrorsOnMultipleOpen(t *testing.T) {
	runner := &fakeRunner{out: `[{"id":"1","state":"OPEN"},{"id":"2","state":"OPEN"}]`}
	client := githubcli.NewWithRunner(runner)

	_, err := client.GetPRForBranch(context.Background(), githubcli.GetPRParams{Branch: "feature"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "multiple open pull requests")
}

func TestGetPRForBranchPropagatesRunnerError(t *testing.T) {
	runner := &fakeRunner{err: errors.New("boom")}
	client := githubcli.NewWithRunner(runner)

	_, err := client.GetPRForBranch(context.Background(), githubcli.GetPRParams{Branch: "feature"})
	require.Error(t, err)
}

func TestGetPRForBranchValidatesBranch(t *testing.T) {
	client := githubcli.NewWithRunner(&fakeRunner{})
	_, err := client.GetPRForBranch(context.Background(), githubcli.GetPRParams{Branch: " "})
	require.Error(t, err)
}

func TestUpdateReviewersValidatesNumber(t *testing.T) {
	client := githubcli.NewWithRunner(&fakeRunner{})
	require.Error(t, client.UpdateReviewers(context.Background(), githubcli.UpdateReviewersParams{}))
}

func TestUpdateReviewersRunsGh(t *testing.T) {
	runner := &fakeRunner{}
	client := githubcli.NewWithRunner(runner)

	params := githubcli.UpdateReviewersParams{
		Number: 42,
		Add:    []string{"alice", " alice ", ""},
		Remove: []string{"bob", "bob"},
	}
	require.NoError(t, client.UpdateReviewers(context.Background(), params))
	require.Equal(t, []string{"pr", "edit", "42", "--add-reviewer", "alice", "--remove-reviewer", "bob"}, runner.args)
}

func TestUpdateReviewersNoop(t *testing.T) {
	runner := &fakeRunner{}
	client := githubcli.NewWithRunner(runner)

	require.NoError(t, client.UpdateReviewers(context.Background(), githubcli.UpdateReviewersParams{Number: 5}))
	require.Empty(t, runner.args)
}

func TestUpdateReviewersPropagatesError(t *testing.T) {
	runner := &fakeRunner{err: errors.New("boom")}
	client := githubcli.NewWithRunner(runner)

	params := githubcli.UpdateReviewersParams{Number: 7, Add: []string{"alice"}}
	require.Error(t, client.UpdateReviewers(context.Background(), params))
}

func TestCreatePRValidatesParams(t *testing.T) {
	client := githubcli.NewWithRunner(&fakeRunner{})
	err := client.CreatePR(context.Background(), githubcli.CreateParams{Head: "", Base: "main"})
	require.Error(t, err)

	err = client.CreatePR(context.Background(), githubcli.CreateParams{Head: "feature", Base: ""})
	require.Error(t, err)
}

func TestCreatePRRunsGh(t *testing.T) {
	runner := &fakeRunner{}
	client := githubcli.NewWithRunner(runner)
	require.NoError(t, client.CreatePR(context.Background(), githubcli.CreateParams{Head: "feature", Base: "main"}))
	require.Equal(t, []string{"pr", "create", "--head", "feature", "--base", "main", "--fill"}, runner.args)
}

func TestEditPRBaseValidatesParams(t *testing.T) {
	client := githubcli.NewWithRunner(&fakeRunner{})
	require.Error(t, client.EditPRBase(context.Background(), 0, "main"))
	require.Error(t, client.EditPRBase(context.Background(), 3, ""))
}

func TestEditPRBaseRunsGh(t *testing.T) {
	runner := &fakeRunner{}
	client := githubcli.NewWithRunner(runner)
	require.NoError(t, client.EditPRBase(context.Background(), 42, "develop"))
	require.Equal(t, []string{"pr", "edit", "42", "--base", "develop"}, runner.args)
}

func TestEditPRBodyValidatesNumber(t *testing.T) {
	client := githubcli.NewWithRunner(&fakeRunner{})
	require.Error(t, client.EditPRBody(context.Background(), 0, "body"))
}

func TestEditPRBodyRunsGh(t *testing.T) {
	runner := &fakeRunner{}
	client := githubcli.NewWithRunner(runner)
	require.NoError(t, client.EditPRBody(context.Background(), 101, "new body"))
	require.Equal(t, []string{"pr", "edit", "101", "--body", "new body"}, runner.args)
}

func TestMergePRValidatesParams(t *testing.T) {
	client := githubcli.NewWithRunner(&fakeRunner{})
	require.Error(t, client.MergePR(context.Background(), githubcli.MergeParams{Head: "", MatchHead: "abc"}))
	require.Error(t, client.MergePR(context.Background(), githubcli.MergeParams{Head: "feature", MatchHead: ""}))
}

func TestMergePRRunsGh(t *testing.T) {
	runner := &fakeRunner{}
	client := githubcli.NewWithRunner(runner)
	require.NoError(t, client.MergePR(context.Background(), githubcli.MergeParams{Head: "feature", MatchHead: "deadbeef", Auto: true}))
	require.Equal(t, []string{"pr", "merge", "feature", "--squash", "--match-head-commit", "deadbeef", "--auto"}, runner.args)
}
