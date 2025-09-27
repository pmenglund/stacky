package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/pmenglund/stacky/internal/githubcli"
)

type inboxEngine interface {
	AuthoredPullRequests(ctx context.Context) ([]githubcli.PullRequest, error)
	ReviewRequestedPullRequests(ctx context.Context) ([]githubcli.PullRequest, error)
}

func newInboxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inbox",
		Short: "List all active GitHub pull requests for the current user",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			eng, err := newEngine(ctx)
			if err != nil {
				return err
			}

			compact, _ := cmd.Flags().GetBool("compact")
			out, err := renderInbox(ctx, eng, compact)
			if err != nil {
				return err
			}

			fmt.Fprint(cmd.OutOrStdout(), out)
			return nil
		},
	}
	cmd.Flags().BoolP("compact", "c", false, "show compact view")
	return cmd
}

func newPrsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "prs",
		Short: "Interactive PR management",
		RunE:  notImplemented("prs"),
	}
}

func renderInbox(ctx context.Context, eng inboxEngine, compact bool) (string, error) {
	authored, err := eng.AuthoredPullRequests(ctx)
	if err != nil {
		return "", err
	}
	review, err := eng.ReviewRequestedPullRequests(ctx)
	if err != nil {
		return "", err
	}

	waitOnMe, waitOnReview, approved := categorizeAuthored(authored)
	sortPRs(waitOnMe)
	sortPRs(waitOnReview)
	sortPRs(approved)
	sortPRs(review)

	var b strings.Builder

	if len(waitOnMe) > 0 {
		b.WriteString("Your PRs - Waiting on You:\n")
		formatPRList(&b, waitOnMe, compact, false)
		b.WriteString("\n")
	}

	if len(waitOnReview) > 0 {
		b.WriteString("Your PRs - Waiting on Review:\n")
		formatPRList(&b, waitOnReview, compact, false)
		b.WriteString("\n")
	}

	if len(approved) > 0 {
		b.WriteString("Your PRs - Approved:\n")
		formatPRList(&b, approved, compact, false)
		b.WriteString("\n")
	}

	if len(waitOnMe)+len(waitOnReview)+len(approved) == 0 {
		b.WriteString("No active pull requests authored by you.\n\n")
	}

	if len(review) > 0 {
		b.WriteString("Pull Requests Awaiting Your Review:\n")
		formatPRList(&b, review, compact, true)
	} else {
		b.WriteString("No pull requests awaiting your review.\n")
	}

	return b.String(), nil
}

func categorizeAuthored(prs []githubcli.PullRequest) (waitingOnMe, waitingOnReview, approved []githubcli.PullRequest) {
	for _, pr := range prs {
		if pr.IsDraft {
			waitingOnMe = append(waitingOnMe, pr)
			continue
		}
		if strings.EqualFold(pr.ReviewDecision, "APPROVED") {
			approved = append(approved, pr)
			continue
		}
		if len(pr.ReviewRequests) > 0 {
			waitingOnReview = append(waitingOnReview, pr)
			continue
		}
		waitingOnMe = append(waitingOnMe, pr)
	}
	return
}

func sortPRs(prs []githubcli.PullRequest) {
	sort.Slice(prs, func(i, j int) bool {
		ti := parseTime(prs[i].UpdatedAt)
		tj := parseTime(prs[j].UpdatedAt)
		return ti.After(tj)
	})
}

func parseTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}
	}
	return t
}

func formatPRList(b *strings.Builder, prs []githubcli.PullRequest, compact bool, showAuthor bool) {
	for _, pr := range prs {
		if compact {
			fmt.Fprintf(b, "- #%d %s (%s -> %s)", pr.Number, pr.Title, pr.HeadRef, pr.BaseRef)
			if showAuthor && pr.Author.Login != "" {
				fmt.Fprintf(b, " by %s", pr.Author.Login)
			}
			if pr.UpdatedAt != "" {
				fmt.Fprintf(b, " [updated %s]", shortDate(pr.UpdatedAt))
			}
			b.WriteString("\n")
			continue
		}

		fmt.Fprintf(b, "#%d %s\n", pr.Number, pr.Title)
		fmt.Fprintf(b, "  %s -> %s\n", pr.HeadRef, pr.BaseRef)
		if showAuthor && pr.Author.Login != "" {
			fmt.Fprintf(b, "  Author: %s\n", pr.Author.Login)
		}
		if pr.URL != "" {
			fmt.Fprintf(b, "  %s\n", pr.URL)
		}
		fmt.Fprintf(b, "  Updated: %s  Created: %s\n\n", shortDate(pr.UpdatedAt), shortDate(pr.CreatedAt))
	}
}

func shortDate(value string) string {
	if value == "" {
		return ""
	}
	if len(value) >= 10 {
		return value[:10]
	}
	return value
}
