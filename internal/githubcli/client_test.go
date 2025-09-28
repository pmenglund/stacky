package githubcli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cli/go-gh/v2/pkg/repository"
	"github.com/pmenglund/stacky/internal/githubcli"
)

type restCall struct {
	Method string
	Path   string
	Body   string
}

type fakeREST struct {
	calls     []restCall
	responses []string
	err       error
}

func (f *fakeREST) DoWithContext(_ context.Context, method, path string, body io.Reader, response interface{}) error {
	var buf bytes.Buffer
	if body != nil {
		_, _ = io.Copy(&buf, body)
	}
	f.calls = append(f.calls, restCall{Method: method, Path: path, Body: buf.String()})
	if f.err != nil {
		return f.err
	}
	if len(f.responses) == 0 || response == nil {
		if len(f.responses) > 0 {
			f.responses = f.responses[1:]
		}
		return nil
	}
	raw := f.responses[0]
	f.responses = f.responses[1:]
	if raw == "" {
		return nil
	}
	return json.Unmarshal([]byte(raw), response)
}

type graphQLCall struct {
	Query     string
	Variables map[string]interface{}
}

type fakeGraphQL struct {
	calls     []graphQLCall
	responses []string
	err       error
}

func (f *fakeGraphQL) DoWithContext(_ context.Context, query string, variables map[string]interface{}, response interface{}) error {
	copied := make(map[string]interface{}, len(variables))
	for k, v := range variables {
		copied[k] = v
	}
	f.calls = append(f.calls, graphQLCall{Query: query, Variables: copied})
	if f.err != nil {
		return f.err
	}
	if len(f.responses) == 0 {
		return nil
	}
	raw := f.responses[0]
	f.responses = f.responses[1:]
	if raw == "" || response == nil {
		return nil
	}
	return json.Unmarshal([]byte(raw), response)
}

var testRepo = repository.Repository{Owner: "octo", Name: "repo", Host: "github.com"}

func TestListPRsParsesResponse(t *testing.T) {
	gql := &fakeGraphQL{responses: []string{`{
  "repository": {
    "pullRequests": {
      "nodes": [
        {
          "id": "abc",
          "number": 1,
          "state": "OPEN",
          "url": "https://example.com",
          "title": "Title",
          "baseRefName": "main",
          "headRefName": "feature",
          "mergeable": "MERGEABLE",
          "mergeStateStatus": "CLEAN",
          "updatedAt": "2024-04-07T00:00:00Z",
          "createdAt": "2024-04-01T00:00:00Z",
          "reviewDecision": "APPROVED",
          "isDraft": false,
          "author": {"login": "octo"},
          "reviewRequests": {
            "nodes": [{"requestedReviewer": {"login": "reviewer"}}]
          },
          "statusCheckRollup": {"state": "SUCCESS"},
          "body": "body"
        }
      ],
      "pageInfo": {"hasNextPage": false, "endCursor": ""}
    }
  }
}`}}
	client := githubcli.NewWithClients(&fakeREST{}, gql, testRepo)

	prs, err := client.ListPRs(context.Background(), githubcli.ListParams{})
	require.NoError(t, err)
	require.Len(t, prs, 1)
	require.Equal(t, "feature", prs[0].HeadRef)
	require.Equal(t, "reviewer", prs[0].ReviewRequests[0].Login)
	require.Equal(t, "SUCCESS", prs[0].StatusCheckRollup[0].State)

	require.NotEmpty(t, gql.calls)
	call := gql.calls[0]
	require.Contains(t, call.Query, "repository")
	require.Equal(t, "octo", call.Variables["owner"])
	require.Equal(t, "repo", call.Variables["name"])
	states, ok := call.Variables["states"].([]string)
	require.True(t, ok)
	require.ElementsMatch(t, []string{"OPEN"}, states)
}

func TestListPRsIncludesExtraFields(t *testing.T) {
	gql := &fakeGraphQL{responses: []string{`{
  "repository": {
    "pullRequests": {
      "nodes": [
        {
          "id": "def",
          "number": 2,
          "state": "OPEN",
          "url": "https://example.com/pr/2",
          "title": "Second",
          "baseRefName": "main",
          "headRefName": "feature2",
          "mergeable": "MERGEABLE",
          "mergeStateStatus": "CLEAN",
          "updatedAt": "2024-04-08T00:00:00Z",
          "createdAt": "2024-04-02T00:00:00Z",
          "reviewDecision": "REVIEW_REQUIRED",
          "isDraft": false,
          "author": {"login": "octo"},
          "reviewRequests": {"nodes": []},
          "statusCheckRollup": null,
          "body": "body",
          "commits": {"nodes": [{"commit": {"oid": "deadbeef"}}]}
        }
      ],
      "pageInfo": {"hasNextPage": false, "endCursor": ""}
    }
  }
}`}}
	client := githubcli.NewWithClients(&fakeREST{}, gql, testRepo)

	prs, err := client.ListPRs(context.Background(), githubcli.ListParams{Head: "feature2", Fields: []string{"commits"}})
	require.NoError(t, err)
	require.Len(t, prs, 1)
	require.Equal(t, "deadbeef", prs[0].Commits[0].OID)

	require.Contains(t, gql.calls[0].Query, "commits")
	require.Equal(t, "feature2", gql.calls[0].Variables["headRefName"])
}

func TestListPRsWithSearchBuildsQuery(t *testing.T) {
	gql := &fakeGraphQL{responses: []string{`{
  "search": {
    "nodes": [],
    "pageInfo": {"hasNextPage": false, "endCursor": ""}
  }
}`}}
	client := githubcli.NewWithClients(&fakeREST{}, gql, testRepo)

	_, err := client.ListPRs(context.Background(), githubcli.ListParams{Search: "review-requested:@me", Author: "octocat"})
	require.NoError(t, err)

	require.Len(t, gql.calls, 1)
	vars := gql.calls[0].Variables
	query, ok := vars["query"].(string)
	require.True(t, ok)
	require.Contains(t, query, "review-requested:@me")
	require.Contains(t, query, "author:octocat")
	require.Contains(t, query, "repo:octo/repo")
	require.Contains(t, query, "sort:updated-desc")
}

func TestGetPRForBranchParsesFixture(t *testing.T) {
	fixturePath := filepath.Join("testdata", "branch_prs_graphql.json")
	data, err := os.ReadFile(fixturePath)
	require.NoError(t, err)

	gql := &fakeGraphQL{responses: []string{string(data)}}
	client := githubcli.NewWithClients(&fakeREST{}, gql, testRepo)

	prs, err := client.GetPRForBranch(context.Background(), githubcli.GetPRParams{Branch: "feature", Fields: []string{"commits"}})
	require.NoError(t, err)
	require.NotNil(t, prs.Open)
	require.Equal(t, "PR_kwABC123", prs.Open.ID)
	require.Equal(t, 1, len(prs.All))

	call := gql.calls[0]
	require.Equal(t, "feature", call.Variables["headRefName"])
	states, ok := call.Variables["states"].([]string)
	require.True(t, ok)
	require.Contains(t, states, "OPEN")
	require.Contains(t, states, "CLOSED")
	require.Contains(t, states, "MERGED")
}

func TestGetPRForBranchErrorsOnMultipleOpen(t *testing.T) {
	gql := &fakeGraphQL{responses: []string{`{
  "repository": {
    "pullRequests": {
      "nodes": [
        {"id": "1", "state": "OPEN", "number": 1, "url": "", "title": "", "baseRefName": "", "headRefName": "feature", "mergeable": "", "mergeStateStatus": "", "updatedAt": "", "createdAt": "", "reviewDecision": "", "isDraft": false, "author": {"login": ""}, "reviewRequests": {"nodes": []}, "statusCheckRollup": null, "body": ""},
        {"id": "2", "state": "OPEN", "number": 2, "url": "", "title": "", "baseRefName": "", "headRefName": "feature", "mergeable": "", "mergeStateStatus": "", "updatedAt": "", "createdAt": "", "reviewDecision": "", "isDraft": false, "author": {"login": ""}, "reviewRequests": {"nodes": []}, "statusCheckRollup": null, "body": ""}
      ],
      "pageInfo": {"hasNextPage": false, "endCursor": ""}
    }
  }
}`}}
	client := githubcli.NewWithClients(&fakeREST{}, gql, testRepo)

	_, err := client.GetPRForBranch(context.Background(), githubcli.GetPRParams{Branch: "feature"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "multiple open pull requests")
}

func TestUpdateReviewersSendsRequests(t *testing.T) {
	rest := &fakeREST{}
	gql := &fakeGraphQL{}
	client := githubcli.NewWithClients(rest, gql, testRepo)

	params := githubcli.UpdateReviewersParams{
		Number: 42,
		Add:    []string{"alice", " alice ", ""},
		Remove: []string{"bob", "bob"},
	}
	require.NoError(t, client.UpdateReviewers(context.Background(), params))

	require.Len(t, rest.calls, 2)
	require.Equal(t, http.MethodPost, rest.calls[0].Method)
	require.Contains(t, rest.calls[0].Body, "alice")
	require.Equal(t, http.MethodDelete, rest.calls[1].Method)
	require.Contains(t, rest.calls[1].Body, "bob")
}

func TestUpdateReviewersNoop(t *testing.T) {
	client := githubcli.NewWithClients(&fakeREST{}, &fakeGraphQL{}, testRepo)
	require.NoError(t, client.UpdateReviewers(context.Background(), githubcli.UpdateReviewersParams{Number: 5}))
}

func TestCreatePRValidatesParams(t *testing.T) {
	client := githubcli.NewWithClients(&fakeREST{}, &fakeGraphQL{}, testRepo)
	err := client.CreatePR(context.Background(), githubcli.CreateParams{Head: "", Base: "main"})
	require.Error(t, err)
	err = client.CreatePR(context.Background(), githubcli.CreateParams{Head: "feature", Base: ""})
	require.Error(t, err)
}

func TestCreatePRSendsRequest(t *testing.T) {
	rest := &fakeREST{}
	client := githubcli.NewWithClients(rest, &fakeGraphQL{}, testRepo)

	require.NoError(t, client.CreatePR(context.Background(), githubcli.CreateParams{Head: "feature", Base: "main"}))
	require.Len(t, rest.calls, 1)
	call := rest.calls[0]
	require.Equal(t, http.MethodPost, call.Method)
	require.Equal(t, "/repos/octo/repo/pulls", call.Path)
	require.Contains(t, call.Body, "\"head\":\"feature\"")
	require.Contains(t, call.Body, "\"base\":\"main\"")
}

func TestEditPRBaseAndBodySendRequests(t *testing.T) {
	rest := &fakeREST{}
	client := githubcli.NewWithClients(rest, &fakeGraphQL{}, testRepo)

	require.NoError(t, client.EditPRBase(context.Background(), 42, "develop"))
	require.NoError(t, client.EditPRBody(context.Background(), 42, "new body"))
	require.Len(t, rest.calls, 2)
	require.Equal(t, http.MethodPatch, rest.calls[0].Method)
	require.Contains(t, rest.calls[0].Body, "develop")
	require.Equal(t, http.MethodPatch, rest.calls[1].Method)
	require.Contains(t, rest.calls[1].Body, "new body")
}

func TestMergePRRequiresParams(t *testing.T) {
	client := githubcli.NewWithClients(&fakeREST{}, &fakeGraphQL{}, testRepo)
	require.Error(t, client.MergePR(context.Background(), githubcli.MergeParams{Head: "", MatchHead: "abc"}))
	require.Error(t, client.MergePR(context.Background(), githubcli.MergeParams{Head: "feature", MatchHead: ""}))
}

func TestMergePRCallsREST(t *testing.T) {
	gql := &fakeGraphQL{responses: []string{`{
  "repository": {
    "pullRequests": {
      "nodes": [
        {"id": "PR1", "number": 7, "state": "OPEN", "url": "", "title": "", "baseRefName": "main", "headRefName": "feature", "mergeable": "MERGEABLE", "mergeStateStatus": "CLEAN", "updatedAt": "", "createdAt": "", "reviewDecision": "", "isDraft": false, "author": {"login": ""}, "reviewRequests": {"nodes": []}, "statusCheckRollup": null, "body": ""}
      ],
      "pageInfo": {"hasNextPage": false, "endCursor": ""}
    }
  }
}`, `{"enablePullRequestAutoMerge":{"pullRequest":{"id":"PR1"}}}`}}
	rest := &fakeREST{responses: []string{`{"merged": true}`}}
	client := githubcli.NewWithClients(rest, gql, testRepo)

	require.NoError(t, client.MergePR(context.Background(), githubcli.MergeParams{Head: "feature", MatchHead: "deadbeef"}))
	require.Len(t, rest.calls, 1)
	require.Equal(t, http.MethodPut, rest.calls[0].Method)
	require.Contains(t, rest.calls[0].Body, "deadbeef")

	// Auto-merge path
	rest.calls = nil
	gql.responses = []string{`{
  "repository": {
    "pullRequests": {
      "nodes": [
        {"id": "PR1", "number": 7, "state": "OPEN", "url": "", "title": "", "baseRefName": "main", "headRefName": "feature", "mergeable": "MERGEABLE", "mergeStateStatus": "CLEAN", "updatedAt": "", "createdAt": "", "reviewDecision": "", "isDraft": false, "author": {"login": ""}, "reviewRequests": {"nodes": []}, "statusCheckRollup": null, "body": ""}
      ],
      "pageInfo": {"hasNextPage": false, "endCursor": ""}
    }
  }
}`, `{"enablePullRequestAutoMerge":{"pullRequest":{"id":"PR1"}}}`}

	require.NoError(t, client.MergePR(context.Background(), githubcli.MergeParams{Head: "feature", MatchHead: "deadbeef", Auto: true}))
	require.Empty(t, rest.calls)
	require.GreaterOrEqual(t, len(gql.calls), 2)
	require.Contains(t, gql.calls[len(gql.calls)-1].Query, "enablePullRequestAutoMerge")
}

func TestMergePRRequiresOpenPR(t *testing.T) {
	gql := &fakeGraphQL{responses: []string{`{
  "repository": {
    "pullRequests": {
      "nodes": [],
      "pageInfo": {"hasNextPage": false, "endCursor": ""}
    }
  }
}`}}
	client := githubcli.NewWithClients(&fakeREST{}, gql, testRepo)

	err := client.MergePR(context.Background(), githubcli.MergeParams{Head: "feature", MatchHead: "deadbeef"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not have an open pull request")
}

func TestEnsureInitializedValidatesRepository(t *testing.T) {
	client := githubcli.New()
	_, err := client.ListPRs(context.Background(), githubcli.ListParams{})
	require.Error(t, err)
}
