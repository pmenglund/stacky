package cmd

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pmenglund/stacky/internal/githubcli"
)

type fakeInboxEngine struct {
	authored []githubcli.PullRequest
	review   []githubcli.PullRequest
	err      error
}

func (f *fakeInboxEngine) AuthoredPullRequests(context.Context) ([]githubcli.PullRequest, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.authored, nil
}

func (f *fakeInboxEngine) ReviewRequestedPullRequests(context.Context) ([]githubcli.PullRequest, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.review, nil
}

func TestRenderInboxCategorizesPRs(t *testing.T) {
	eng := &fakeInboxEngine{
		authored: []githubcli.PullRequest{
			{Number: 1, Title: "Draft", HeadRef: "feature", BaseRef: "main", UpdatedAt: "2024-04-07T00:00:00Z", IsDraft: true},
			{Number: 2, Title: "Needs Review", HeadRef: "feature2", BaseRef: "main", UpdatedAt: "2024-04-06T00:00:00Z", ReviewRequests: []struct {
				Login string `json:"login"`
			}{{Login: "reviewer"}}},
			{Number: 3, Title: "Approved", HeadRef: "feature3", BaseRef: "main", UpdatedAt: "2024-04-05T00:00:00Z", ReviewDecision: "APPROVED"},
		},
		review: []githubcli.PullRequest{
			{Number: 4, Title: "Needs Your Review", HeadRef: "other", BaseRef: "main", UpdatedAt: "2024-04-04T00:00:00Z", Author: struct {
				Login string `json:"login"`
			}{Login: "octocat"}},
		},
	}

	out, err := renderInbox(context.Background(), eng, true)
	require.NoError(t, err)
	require.Contains(t, out, "Your PRs - Waiting on You:")
	require.Contains(t, out, "Your PRs - Waiting on Review:")
	require.Contains(t, out, "Your PRs - Approved:")
	require.Contains(t, out, "Pull Requests Awaiting Your Review:")
	require.Contains(t, out, "#1")
	require.Contains(t, out, "#4")
}

func TestRenderInboxNoPRs(t *testing.T) {
	eng := &fakeInboxEngine{}
	out, err := renderInbox(context.Background(), eng, true)
	require.NoError(t, err)
	require.Contains(t, out, "No active pull requests authored by you")
	require.Contains(t, out, "No pull requests awaiting your review")
}

func TestRenderInboxPropagatesError(t *testing.T) {
	eng := &fakeInboxEngine{err: errors.New("boom")}
	_, err := renderInbox(context.Background(), eng, false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "boom")
}
