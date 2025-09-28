package cmd

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/spf13/cobra"

	"github.com/pmenglund/stacky/internal/githubcli"
	"github.com/pmenglund/stacky/internal/ui"
)

const (
	colorRed    = "196"
	colorYellow = "226"
	colorGreen  = "46"
	colorBlue   = "33"
	colorCyan   = "81"
	colorWhite  = "15"
	colorGray   = "244"
	colorOrange = "208"
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
			mode := ui.ResolveColorMode(ui.ColorMode(colorMode), cmd.OutOrStdout())
			out, err := renderInbox(ctx, eng, compact, mode)
			if err != nil {
				return err
			}

			cmd.Print(out)
			return nil
		},
	}
	cmd.Flags().BoolP("compact", "c", false, "show compact view")
	return cmd
}

func renderInbox(ctx context.Context, eng inboxEngine, compact bool, mode ui.ColorMode) (string, error) {
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

	formatter := newInboxFormatter(mode, compact)

	if len(waitOnMe) > 0 {
		formatter.section("Your PRs - Waiting on You:", colorRed, waitOnMe, false, true)
	}
	if len(waitOnReview) > 0 {
		formatter.section("Your PRs - Waiting on Review:", colorYellow, waitOnReview, false, true)
	}
	if len(approved) > 0 {
		formatter.section("Your PRs - Approved:", colorGreen, approved, false, true)
	}

	if len(waitOnMe)+len(waitOnReview)+len(approved) == 0 {
		formatter.writeStyled("No active pull requests authored by you.\n", colorGreen, false)
	}

	if len(review) > 0 {
		formatter.section("Pull Requests Awaiting Your Review:", colorYellow, review, true, false)
	} else {
		formatter.writeStyled("No pull requests awaiting your review.\n", colorYellow, false)
	}

	return formatter.String(), nil
}

type inboxFormatter struct {
	builder   strings.Builder
	compact   bool
	renderer  *lipgloss.Renderer
	colorless bool
	hyperlink bool
}

func newInboxFormatter(mode ui.ColorMode, compact bool) *inboxFormatter {
	renderer := lipgloss.NewRenderer(io.Discard)
	formatter := &inboxFormatter{compact: compact, renderer: renderer}
	if mode == ui.ColorModeNever {
		renderer.SetColorProfile(termenv.Ascii)
		formatter.colorless = true
		formatter.hyperlink = false
	} else {
		renderer.SetColorProfile(termenv.TrueColor)
		formatter.hyperlink = true
	}
	return formatter
}

func (f *inboxFormatter) section(title string, color string, prs []githubcli.PullRequest, showAuthor bool, addBlank bool) {
	f.writeHeading(title, color)
	f.renderList(prs, showAuthor)
	if addBlank {
		f.blankLine()
	}
}

func (f *inboxFormatter) writeHeading(text, color string) {
	f.writeStyled(text+"\n", color, true)
}

func (f *inboxFormatter) renderList(prs []githubcli.PullRequest, showAuthor bool) {
	for _, pr := range prs {
		if f.compact {
			f.renderCompact(pr, showAuthor)
			continue
		}
		f.renderFull(pr, showAuthor)
	}
}

func (f *inboxFormatter) renderCompact(pr githubcli.PullRequest, showAuthor bool) {
	f.write(f.renderPRNumber(pr))
	if pr.Title != "" {
		f.write(" ")
		f.writeStyled(pr.Title, colorWhite, false)
	}
	if pr.HeadRef != "" {
		f.write(" ")
		f.writeStyled(fmt.Sprintf("(%s)", pr.HeadRef), colorGray, false)
	}
	if showAuthor && pr.Author.Login != "" {
		f.write(" ")
		f.writeStyled("by "+pr.Author.Login, colorGray, false)
	}
	if pr.IsDraft {
		f.write(" ")
		f.writeStyled("[DRAFT]", colorOrange, false)
	}
	if text, col := checkStatus(pr); text != "" {
		f.write(" ")
		f.writeStyled(text, col, false)
	}
	if date := shortDate(pr.UpdatedAt); date != "" {
		f.write(" ")
		f.writeStyled("Updated:", colorGray, false)
		f.write(" ")
		f.writeStyled(date, colorGray, false)
	}
	f.write("\n")
}

func (f *inboxFormatter) renderFull(pr githubcli.PullRequest, showAuthor bool) {
	f.write(f.renderPRNumber(pr))
	if pr.Title != "" {
		f.write(" ")
		f.writeStyled(pr.Title, colorWhite, false)
	}
	f.write("\n")

	f.writeStyled(fmt.Sprintf("  %s -> %s\n", pr.HeadRef, pr.BaseRef), colorGray, false)
	if showAuthor && pr.Author.Login != "" {
		f.writeStyled(fmt.Sprintf("  Author: %s\n", pr.Author.Login), colorGray, false)
	}
	if pr.IsDraft {
		f.writeStyled("  [DRAFT]\n", colorOrange, false)
	}
	if text, col := checkStatus(pr); text != "" {
		f.writeStyled("  "+text+"\n", col, false)
	}
	if pr.URL != "" {
		f.write("  ")
		f.write(f.renderURL(pr.URL))
		f.write("\n")
	}
	f.writeStyled(fmt.Sprintf("  Updated: %s, Created: %s\n", shortDate(pr.UpdatedAt), shortDate(pr.CreatedAt)), colorGray, false)
	f.write("\n")
}

func (f *inboxFormatter) renderPRNumber(pr githubcli.PullRequest) string {
	label := fmt.Sprintf("#%d", pr.Number)
	styled := f.styled(label, colorCyan, false)
	if f.hyperlink && pr.URL != "" {
		return hyperlink(pr.URL, styled)
	}
	return styled
}

func (f *inboxFormatter) renderURL(url string) string {
	styled := f.styled(url, colorBlue, false)
	if f.hyperlink && url != "" {
		return hyperlink(url, styled)
	}
	return styled
}

func (f *inboxFormatter) styled(text, color string, bold bool) string {
	if text == "" {
		return ""
	}
	if f.colorless {
		return text
	}
	style := f.renderer.NewStyle()
	if color != "" {
		style = style.Foreground(lipgloss.Color(color))
	}
	if bold {
		style = style.Bold(true)
	}
	return style.Render(text)
}

func (f *inboxFormatter) writeStyled(text, color string, bold bool) {
	if text == "" {
		return
	}
	if strings.HasSuffix(text, "\n") {
		withoutNewline := strings.TrimSuffix(text, "\n")
		if withoutNewline != "" {
			f.write(f.styled(withoutNewline, color, bold))
		}
		f.write("\n")
		return
	}
	f.write(f.styled(text, color, bold))
}

func (f *inboxFormatter) write(text string) {
	if text == "" {
		return
	}
	f.builder.WriteString(text)
}

func (f *inboxFormatter) blankLine() {
	f.write("\n")
}

func (f *inboxFormatter) String() string {
	return f.builder.String()
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

func shortDate(value string) string {
	if value == "" {
		return ""
	}
	if len(value) >= 10 {
		return value[:10]
	}
	return value
}

func checkStatus(pr githubcli.PullRequest) (string, string) {
	if len(pr.StatusCheckRollup) == 0 {
		return "", ""
	}
	states := make([]string, 0, len(pr.StatusCheckRollup))
	for _, rollup := range pr.StatusCheckRollup {
		state := strings.ToUpper(strings.TrimSpace(rollup.State))
		if state != "" {
			states = append(states, state)
		}
	}
	if len(states) == 0 {
		return "", ""
	}

	if containsState(states, "FAILURE", "ERROR") {
		return "✗ Checks failed", colorRed
	}
	if containsState(states, "PENDING", "QUEUED", "IN_PROGRESS") {
		return "⏳ Checks running", colorYellow
	}
	if allStates(states, "SUCCESS") {
		return "✓ Checks passed", colorGreen
	}
	return "Checks mixed", colorYellow
}

func containsState(states []string, targets ...string) bool {
	for _, state := range states {
		for _, target := range targets {
			if state == target {
				return true
			}
		}
	}
	return false
}

func allStates(states []string, value string) bool {
	if len(states) == 0 {
		return false
	}
	for _, state := range states {
		if state != value {
			return false
		}
	}
	return true
}

func hyperlink(url, text string) string {
	if url == "" || text == "" {
		return text
	}
	return fmt.Sprintf("\033]8;;%s\033\\%s\033]8;;\033\\", url, text)
}
