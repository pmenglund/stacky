package cmd

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pmenglund/stacky/internal/githubcli"
	"github.com/pmenglund/stacky/internal/ui"
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
			{
				Number:    1,
				Title:     "Draft",
				HeadRef:   "feature",
				BaseRef:   "main",
				URL:       "https://example.com/pr/1",
				UpdatedAt: "2024-04-07T00:00:00Z",
				CreatedAt: "2024-04-01T00:00:00Z",
				IsDraft:   true,
				StatusCheckRollup: []struct {
					State string `json:"state"`
				}{{State: "PENDING"}},
			},
			{
				Number:    2,
				Title:     "Needs Review",
				HeadRef:   "feature2",
				BaseRef:   "main",
				URL:       "https://example.com/pr/2",
				UpdatedAt: "2024-04-06T00:00:00Z",
				ReviewRequests: []struct {
					Login string `json:"login"`
				}{{Login: "reviewer"}},
				StatusCheckRollup: []struct {
					State string `json:"state"`
				}{{State: "SUCCESS"}},
			},
			{
				Number:         3,
				Title:          "Approved",
				HeadRef:        "feature3",
				BaseRef:        "main",
				URL:            "https://example.com/pr/3",
				UpdatedAt:      "2024-04-05T00:00:00Z",
				ReviewDecision: "APPROVED",
			},
		},
		review: []githubcli.PullRequest{
			{
				Number:    4,
				Title:     "Needs Your Review",
				HeadRef:   "other",
				BaseRef:   "main",
				URL:       "https://example.com/pr/4",
				UpdatedAt: "2024-04-04T00:00:00Z",
				CreatedAt: "2024-04-02T00:00:00Z",
				Author: struct {
					Login string `json:"login"`
				}{Login: "octocat"},
				StatusCheckRollup: []struct {
					State string `json:"state"`
				}{{State: "FAILURE"}},
			},
		},
	}

	out, err := renderInbox(context.Background(), eng, true, ui.ColorModeNever)
	require.NoError(t, err)
	require.Contains(t, out, "Your PRs - Waiting on You:")
	require.Contains(t, out, "Your PRs - Waiting on Review:")
	require.Contains(t, out, "Your PRs - Approved:")
	require.Contains(t, out, "Pull Requests Awaiting Your Review:")
	require.Contains(t, out, "#1")
	require.Contains(t, out, "#4")
	require.Contains(t, out, "[DRAFT]")
	require.Contains(t, out, "⏳ Checks running")
	require.Contains(t, out, "✗ Checks failed")
	require.Contains(t, out, "Updated: 2024-04-07")
	require.NotContains(t, out, "\u001b]8;;")
}

func TestRenderInboxNoPRs(t *testing.T) {
	eng := &fakeInboxEngine{}
	out, err := renderInbox(context.Background(), eng, true, ui.ColorModeNever)
	require.NoError(t, err)
	require.Contains(t, out, "No active pull requests authored by you")
	require.Contains(t, out, "No pull requests awaiting your review")
}

func TestRenderInboxPropagatesError(t *testing.T) {
	eng := &fakeInboxEngine{err: errors.New("boom")}
	_, err := renderInbox(context.Background(), eng, false, ui.ColorModeNever)
	require.Error(t, err)
	require.Contains(t, err.Error(), "boom")
}

func TestRenderInboxFullFormatShowsDetails(t *testing.T) {
	eng := &fakeInboxEngine{
		authored: []githubcli.PullRequest{{
			Number:    10,
			Title:     "Full View",
			HeadRef:   "feature/full",
			BaseRef:   "main",
			URL:       "https://example.com/pr/10",
			CreatedAt: "2024-04-01T00:00:00Z",
			UpdatedAt: "2024-04-08T00:00:00Z",
			StatusCheckRollup: []struct {
				State string `json:"state"`
			}{{State: "SUCCESS"}},
		}},
		review: []githubcli.PullRequest{{
			Number:  11,
			Title:   "Review me",
			HeadRef: "other/full",
			BaseRef: "main",
			Author: struct {
				Login string `json:"login"`
			}{Login: "octocat"},
			CreatedAt: "2024-04-03T00:00:00Z",
			UpdatedAt: "2024-04-04T00:00:00Z",
		}},
	}

	out, err := renderInbox(context.Background(), eng, false, ui.ColorModeNever)
	require.NoError(t, err)
	require.Contains(t, out, "feature/full -> main")
	require.Contains(t, out, "Author: octocat")
	require.Contains(t, out, "✓ Checks passed")
}

func TestRenderInboxHyperlinkEnabledWhenColorful(t *testing.T) {
	eng := &fakeInboxEngine{
		authored: []githubcli.PullRequest{{
			Number:    1,
			Title:     "Link",
			HeadRef:   "feature",
			BaseRef:   "main",
			URL:       "https://example.com/pr/1",
			UpdatedAt: "2024-04-07T00:00:00Z",
		}},
	}

	out, err := renderInbox(context.Background(), eng, true, ui.ColorModeAlways)
	require.NoError(t, err)
	require.Contains(t, out, "\u001b]8;;https://example.com/pr/1")
}
