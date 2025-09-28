package githubcli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	api "github.com/cli/go-gh/v2/pkg/api"
	ghrepo "github.com/cli/go-gh/v2/pkg/repository"
)

// Client describes the interactions stacky performs with the GitHub CLI.
type Client interface {
	ListPRs(ctx context.Context, params ListParams) ([]PullRequest, error)
	GetPRForBranch(ctx context.Context, params GetPRParams) (BranchPullRequests, error)
	UpdateReviewers(ctx context.Context, params UpdateReviewersParams) error
	CreatePR(ctx context.Context, params CreateParams) error
	EditPRBase(ctx context.Context, number int, base string) error
	EditPRBody(ctx context.Context, number int, body string) error
	MergePR(ctx context.Context, params MergeParams) error
}

// ListParams captures high-level filters used when listing PRs.
type ListParams struct {
	Author string
	Head   string
	State  string
	Search string
	Fields []string
}

// CreateParams describes the arguments used when opening a new pull request.
type CreateParams struct {
	Head string
	Base string
}

// MergeParams describes how to merge a pull request via the GitHub CLI.
type MergeParams struct {
	Head      string
	MatchHead string
	Auto      bool
}

// GetPRParams controls how branch-specific pull requests are fetched.
type GetPRParams struct {
	Branch string
	Fields []string
}

// BranchPullRequests captures all pull requests for a branch and the single open PR.
type BranchPullRequests struct {
	All  map[string]PullRequest
	Open *PullRequest
}

// UpdateReviewersParams describes reviewer modifications for an existing PR.
type UpdateReviewersParams struct {
	Number int
	Add    []string
	Remove []string
}

// PullRequest mirrors the fields consumed by the CLI presentation.
type PullRequest struct {
	ID               string `json:"id"`
	Number           int    `json:"number"`
	State            string `json:"state"`
	URL              string `json:"url"`
	Title            string `json:"title"`
	BaseRef          string `json:"baseRefName"`
	HeadRef          string `json:"headRefName"`
	Mergeable        string `json:"mergeable"`
	MergeStateStatus string `json:"mergeStateStatus"`
	UpdatedAt        string `json:"updatedAt"`
	CreatedAt        string `json:"createdAt"`
	ReviewDecision   string `json:"reviewDecision"`
	IsDraft          bool   `json:"isDraft"`
	Author           struct {
		Login string `json:"login"`
	} `json:"author"`
	ReviewRequests []struct {
		Login string `json:"login"`
	} `json:"reviewRequests"`
	StatusCheckRollup []struct {
		State string `json:"state"`
	} `json:"statusCheckRollup"`
	Body    string              `json:"body"`
	Commits []PullRequestCommit `json:"commits"`
}

// PullRequestCommit captures the subset of commit metadata required by stacky.
type PullRequestCommit struct {
	OID string `json:"oid"`
}

type restClient interface {
	DoWithContext(ctx context.Context, method, path string, body io.Reader, response interface{}) error
}

type graphQLClient interface {
	DoWithContext(ctx context.Context, query string, variables map[string]interface{}, response interface{}) error
}

type client struct {
	rest    restClient
	gql     graphQLClient
	repo    ghrepo.Repository
	initErr error
}

const (
	pullRequestPageSize    = 100
	reviewRequestsPageSize = 30
	commitPageSize         = 250
)

// New constructs a client using shared REST and GraphQL clients configured via go-gh.
func New() Client {
	rest, restErr := api.DefaultRESTClient()
	gql, gqlErr := api.DefaultGraphQLClient()
	repo, repoErr := ghrepo.Current()

	var initErr error
	for _, err := range []error{restErr, gqlErr, repoErr} {
		if err != nil {
			initErr = errors.Join(initErr, err)
		}
	}

	return &client{
		rest:    rest,
		gql:     gql,
		repo:    repo,
		initErr: initErr,
	}
}

// NewWithClients allows tests to provide fake REST/GraphQL clients and a repository.
func NewWithClients(rest restClient, gql graphQLClient, repo ghrepo.Repository) Client {
	return &client{rest: rest, gql: gql, repo: repo}
}

func (c *client) ensureInitialized() error {
	if c.initErr != nil {
		return c.initErr
	}
	if c.rest == nil || c.gql == nil {
		return fmt.Errorf("githubcli: client not configured")
	}
	if strings.TrimSpace(c.repo.Owner) == "" || strings.TrimSpace(c.repo.Name) == "" {
		return fmt.Errorf("githubcli: repository not resolved")
	}
	return nil
}

func (c *client) ListPRs(ctx context.Context, params ListParams) ([]PullRequest, error) {
	if err := c.ensureInitialized(); err != nil {
		return nil, err
	}

	includeCommits := containsField(params.Fields, "commits")
	var nodes []graphQLPullRequest
	var err error

	if params.Search != "" || params.Author != "" {
		nodes, err = c.listBySearch(ctx, params, includeCommits)
	} else {
		nodes, err = c.listByRepository(ctx, params, includeCommits)
	}
	if err != nil {
		return nil, err
	}

	prs := make([]PullRequest, 0, len(nodes))
	for _, node := range nodes {
		prs = append(prs, convertPullRequest(node, includeCommits))
	}
	return prs, nil
}

func (c *client) GetPRForBranch(ctx context.Context, params GetPRParams) (BranchPullRequests, error) {
	if err := c.ensureInitialized(); err != nil {
		return BranchPullRequests{}, err
	}

	branch := strings.TrimSpace(params.Branch)
	if branch == "" {
		return BranchPullRequests{}, fmt.Errorf("githubcli: branch required")
	}

	includeCommits := containsField(params.Fields, "commits")
	listParams := ListParams{Head: branch, State: "all", Fields: params.Fields}
	nodes, err := c.listByRepository(ctx, listParams, includeCommits)
	if err != nil {
		return BranchPullRequests{}, err
	}

	all := make(map[string]PullRequest, len(nodes))
	var open *PullRequest
	for _, node := range nodes {
		pr := convertPullRequest(node, includeCommits)
		if pr.ID != "" {
			all[pr.ID] = pr
		}
		if strings.EqualFold(pr.State, "OPEN") {
			if open != nil {
				return BranchPullRequests{}, fmt.Errorf("githubcli: branch %s has multiple open pull requests", branch)
			}
			prCopy := pr
			open = &prCopy
		}
	}

	return BranchPullRequests{All: all, Open: open}, nil
}

func (c *client) UpdateReviewers(ctx context.Context, params UpdateReviewersParams) error {
	if err := c.ensureInitialized(); err != nil {
		return err
	}
	if params.Number <= 0 {
		return fmt.Errorf("githubcli: invalid pull request number %d", params.Number)
	}

	add := normalizeReviewers(params.Add)
	remove := normalizeReviewers(params.Remove)

	if len(add) == 0 && len(remove) == 0 {
		return nil
	}

	path := fmt.Sprintf("/repos/%s/%s/pulls/%d/requested_reviewers", c.repo.Owner, c.repo.Name, params.Number)

	if len(add) > 0 {
		body, err := marshalBody(map[string][]string{"reviewers": add})
		if err != nil {
			return err
		}
		if err := c.rest.DoWithContext(ctx, http.MethodPost, path, body, nil); err != nil {
			return fmt.Errorf("githubcli: add reviewers: %w", err)
		}
	}

	if len(remove) > 0 {
		body, err := marshalBody(map[string][]string{"reviewers": remove})
		if err != nil {
			return err
		}
		if err := c.rest.DoWithContext(ctx, http.MethodDelete, path, body, nil); err != nil {
			return fmt.Errorf("githubcli: remove reviewers: %w", err)
		}
	}

	return nil
}

func (c *client) CreatePR(ctx context.Context, params CreateParams) error {
	if err := c.ensureInitialized(); err != nil {
		return err
	}
	if params.Head == "" {
		return fmt.Errorf("githubcli: head branch required")
	}
	if params.Base == "" {
		return fmt.Errorf("githubcli: base branch required")
	}

	payload := map[string]string{
		"head":  params.Head,
		"base":  params.Base,
		"title": params.Head,
		"body":  "",
	}
	body, err := marshalBody(payload)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/repos/%s/%s/pulls", c.repo.Owner, c.repo.Name)
	if err := c.rest.DoWithContext(ctx, http.MethodPost, path, body, nil); err != nil {
		return fmt.Errorf("githubcli: create pull request: %w", err)
	}
	return nil
}

func (c *client) EditPRBase(ctx context.Context, number int, base string) error {
	if err := c.ensureInitialized(); err != nil {
		return err
	}
	if number <= 0 {
		return fmt.Errorf("githubcli: invalid pull request number %d", number)
	}
	if base == "" {
		return fmt.Errorf("githubcli: base branch required")
	}

	payload := map[string]string{"base": base}
	body, err := marshalBody(payload)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/repos/%s/%s/pulls/%d", c.repo.Owner, c.repo.Name, number)
	if err := c.rest.DoWithContext(ctx, http.MethodPatch, path, body, nil); err != nil {
		return fmt.Errorf("githubcli: edit pull request base: %w", err)
	}
	return nil
}

func (c *client) EditPRBody(ctx context.Context, number int, body string) error {
	if err := c.ensureInitialized(); err != nil {
		return err
	}
	if number <= 0 {
		return fmt.Errorf("githubcli: invalid pull request number %d", number)
	}

	payload := map[string]string{"body": body}
	reader, err := marshalBody(payload)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/repos/%s/%s/pulls/%d", c.repo.Owner, c.repo.Name, number)
	if err := c.rest.DoWithContext(ctx, http.MethodPatch, path, reader, nil); err != nil {
		return fmt.Errorf("githubcli: edit pull request body: %w", err)
	}
	return nil
}

func (c *client) MergePR(ctx context.Context, params MergeParams) error {
	if err := c.ensureInitialized(); err != nil {
		return err
	}
	if params.Head == "" {
		return fmt.Errorf("githubcli: head branch required")
	}
	if params.MatchHead == "" {
		return fmt.Errorf("githubcli: head commit required")
	}

	branchPRs, err := c.GetPRForBranch(ctx, GetPRParams{Branch: params.Head})
	if err != nil {
		return err
	}
	if branchPRs.Open == nil {
		return fmt.Errorf("githubcli: branch %s does not have an open pull request", params.Head)
	}

	if params.Auto {
		return c.enableAutoMerge(ctx, branchPRs.Open.ID)
	}

	return c.mergePullRequest(ctx, branchPRs.Open.Number, params.MatchHead)
}

func (c *client) mergePullRequest(ctx context.Context, number int, sha string) error {
	payload := map[string]string{
		"merge_method": "squash",
		"sha":          sha,
	}
	body, err := marshalBody(payload)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/repos/%s/%s/pulls/%d/merge", c.repo.Owner, c.repo.Name, number)
	var resp struct {
		Merged  bool   `json:"merged"`
		Message string `json:"message"`
	}
	if err := c.rest.DoWithContext(ctx, http.MethodPut, path, body, &resp); err != nil {
		return fmt.Errorf("githubcli: merge pull request: %w", err)
	}
	if !resp.Merged {
		if resp.Message != "" {
			return fmt.Errorf("githubcli: merge pull request: %s", resp.Message)
		}
		return fmt.Errorf("githubcli: merge pull request: merge rejected")
	}
	return nil
}

func (c *client) enableAutoMerge(ctx context.Context, prID string) error {
	if strings.TrimSpace(prID) == "" {
		return fmt.Errorf("githubcli: pull request id required for auto-merge")
	}

	query := `mutation EnablePullRequestAutoMerge($pullRequestId: ID!, $mergeMethod: PullRequestMergeMethod!) {
  enablePullRequestAutoMerge(input: {pullRequestId: $pullRequestId, mergeMethod: SQUASH}) {
    pullRequest { id }
  }
}`

	payload := map[string]interface{}{
		"pullRequestId": prID,
		"mergeMethod":   "SQUASH",
	}

	if err := c.gql.DoWithContext(ctx, query, payload, &struct{}{}); err != nil {
		return fmt.Errorf("githubcli: enable auto-merge: %w", err)
	}
	return nil
}

func (c *client) listByRepository(ctx context.Context, params ListParams, includeCommits bool) ([]graphQLPullRequest, error) {
	states, err := parseStates(params.State)
	if err != nil {
		return nil, err
	}
	if len(states) == 0 {
		states = []string{"OPEN"}
	}

	variables := map[string]interface{}{
		"owner":    c.repo.Owner,
		"name":     c.repo.Name,
		"states":   states,
		"pageSize": pullRequestPageSize,
	}
	if strings.TrimSpace(params.Head) != "" {
		variables["headRefName"] = params.Head
	}

	query := buildRepositoryQuery(includeCommits)
	var nodes []graphQLPullRequest

	for {
		var resp repositoryPullRequestResponse
		if err := c.gql.DoWithContext(ctx, query, variables, &resp); err != nil {
			return nil, fmt.Errorf("githubcli: list pull requests: %w", err)
		}
		nodes = append(nodes, resp.Repository.PullRequests.Nodes...)
		if !resp.Repository.PullRequests.PageInfo.HasNextPage {
			break
		}
		variables["after"] = resp.Repository.PullRequests.PageInfo.EndCursor
	}

	return nodes, nil
}

func (c *client) listBySearch(ctx context.Context, params ListParams, includeCommits bool) ([]graphQLPullRequest, error) {
	queryString := c.buildSearchQuery(params)

	variables := map[string]interface{}{
		"query":    queryString,
		"pageSize": pullRequestPageSize,
	}

	query := buildSearchQuery(includeCommits)
	var nodes []graphQLPullRequest

	for {
		var resp searchPullRequestResponse
		if err := c.gql.DoWithContext(ctx, query, variables, &resp); err != nil {
			return nil, fmt.Errorf("githubcli: search pull requests: %w", err)
		}
		nodes = append(nodes, resp.Search.Nodes...)
		if !resp.Search.PageInfo.HasNextPage {
			break
		}
		variables["after"] = resp.Search.PageInfo.EndCursor
	}

	return nodes, nil
}

func (c *client) buildSearchQuery(params ListParams) string {
	tokens := []string{
		fmt.Sprintf("repo:%s/%s", c.repo.Owner, c.repo.Name),
		"type:pr",
		"sort:updated-desc",
	}

	switch strings.ToLower(strings.TrimSpace(params.State)) {
	case "", "open":
		tokens = append(tokens, "is:open")
	case "closed":
		tokens = append(tokens, "is:closed")
	case "merged":
		tokens = append(tokens, "is:merged")
	case "all":
		// no-op to include all states
	default:
		tokens = append(tokens, fmt.Sprintf("state:%s", params.State))
	}

	if strings.TrimSpace(params.Author) != "" {
		tokens = append(tokens, fmt.Sprintf("author:%s", params.Author))
	}
	if strings.TrimSpace(params.Head) != "" {
		tokens = append(tokens, fmt.Sprintf("head:%s", params.Head))
	}
	if params.Search != "" {
		tokens = append(tokens, params.Search)
	}

	return strings.Join(tokens, " ")
}

func parseStates(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{"OPEN"}, nil
	}

	parts := strings.Split(raw, ",")
	states := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.ToUpper(strings.TrimSpace(part))
		switch value {
		case "OPEN", "CLOSED", "MERGED":
			states = append(states, value)
		case "ALL":
			return []string{"OPEN", "CLOSED", "MERGED"}, nil
		case "":
		default:
			return nil, fmt.Errorf("githubcli: unsupported pull request state %s", part)
		}
	}

	if len(states) == 0 {
		return []string{"OPEN"}, nil
	}
	return states, nil
}

func containsField(fields []string, target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	for _, field := range fields {
		if strings.EqualFold(strings.TrimSpace(field), target) {
			return true
		}
	}
	return false
}

func marshalBody(payload interface{}) (io.Reader, error) {
	if payload == nil {
		return nil, nil
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("githubcli: encode request: %w", err)
	}
	return bytes.NewReader(data), nil
}

func buildRepositoryQuery(includeCommits bool) string {
	return fmt.Sprintf(`
query PullRequests($owner: String!, $name: String!, $states: [PullRequestState!], $headRefName: String, $after: String) {
  repository(owner: $owner, name: $name) {
    pullRequests(first: %d, states: $states, headRefName: $headRefName, after: $after, orderBy: {field: UPDATED_AT, direction: DESC}) {
      nodes {
%s
      }
      pageInfo {
        hasNextPage
        endCursor
      }
    }
  }
}
`, pullRequestPageSize, prSelection(includeCommits))
}

func buildSearchQuery(includeCommits bool) string {
	return fmt.Sprintf(`
query SearchPullRequests($query: String!, $after: String) {
  search(query: $query, type: ISSUE, first: %d, after: $after) {
    nodes {
      ... on PullRequest {
%s
      }
    }
    pageInfo {
      hasNextPage
      endCursor
    }
  }
}
`, pullRequestPageSize, prSelection(includeCommits))
}

func prSelection(includeCommits bool) string {
	builder := &strings.Builder{}
	builder.WriteString("        id\n")
	builder.WriteString("        number\n")
	builder.WriteString("        state\n")
	builder.WriteString("        url\n")
	builder.WriteString("        title\n")
	builder.WriteString("        baseRefName\n")
	builder.WriteString("        headRefName\n")
	builder.WriteString("        mergeable\n")
	builder.WriteString("        mergeStateStatus\n")
	builder.WriteString("        updatedAt\n")
	builder.WriteString("        createdAt\n")
	builder.WriteString("        reviewDecision\n")
	builder.WriteString("        isDraft\n")
	builder.WriteString("        author { login }\n")
	builder.WriteString(fmt.Sprintf("        reviewRequests(first: %d) {\n          nodes {\n            requestedReviewer {\n              ... on User { login }\n            }\n          }\n        }\n", reviewRequestsPageSize))
	builder.WriteString("        statusCheckRollup { state }\n")
	builder.WriteString("        body\n")
	if includeCommits {
		builder.WriteString(fmt.Sprintf("        commits(last: %d) {\n          nodes {\n            commit { oid }\n          }\n        }\n", commitPageSize))
	}
	return builder.String()
}

type pageInfo struct {
	HasNextPage bool   `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
}

type repositoryPullRequestResponse struct {
	Repository struct {
		PullRequests struct {
			Nodes    []graphQLPullRequest `json:"nodes"`
			PageInfo pageInfo             `json:"pageInfo"`
		} `json:"pullRequests"`
	} `json:"repository"`
}

type searchPullRequestResponse struct {
	Search struct {
		Nodes    []graphQLPullRequest `json:"nodes"`
		PageInfo pageInfo             `json:"pageInfo"`
	} `json:"search"`
}

type graphQLPullRequest struct {
	ID               string `json:"id"`
	Number           int    `json:"number"`
	State            string `json:"state"`
	URL              string `json:"url"`
	Title            string `json:"title"`
	BaseRefName      string `json:"baseRefName"`
	HeadRefName      string `json:"headRefName"`
	Mergeable        string `json:"mergeable"`
	MergeStateStatus string `json:"mergeStateStatus"`
	UpdatedAt        string `json:"updatedAt"`
	CreatedAt        string `json:"createdAt"`
	ReviewDecision   string `json:"reviewDecision"`
	IsDraft          bool   `json:"isDraft"`
	Author           struct {
		Login string `json:"login"`
	} `json:"author"`
	ReviewRequests struct {
		Nodes []struct {
			RequestedReviewer struct {
				Login string `json:"login"`
			} `json:"requestedReviewer"`
		} `json:"nodes"`
	} `json:"reviewRequests"`
	StatusCheckRollup *struct {
		State string `json:"state"`
	} `json:"statusCheckRollup"`
	Body    string `json:"body"`
	Commits *struct {
		Nodes []struct {
			Commit struct {
				OID string `json:"oid"`
			} `json:"commit"`
		} `json:"nodes"`
	} `json:"commits"`
}

func convertPullRequest(node graphQLPullRequest, includeCommits bool) PullRequest {
	pr := PullRequest{
		ID:               node.ID,
		Number:           node.Number,
		State:            node.State,
		URL:              node.URL,
		Title:            node.Title,
		BaseRef:          node.BaseRefName,
		HeadRef:          node.HeadRefName,
		Mergeable:        node.Mergeable,
		MergeStateStatus: node.MergeStateStatus,
		UpdatedAt:        node.UpdatedAt,
		CreatedAt:        node.CreatedAt,
		ReviewDecision:   node.ReviewDecision,
		IsDraft:          node.IsDraft,
		Author:           node.Author,
		Body:             node.Body,
	}

	for _, rr := range node.ReviewRequests.Nodes {
		login := strings.TrimSpace(rr.RequestedReviewer.Login)
		if login == "" {
			continue
		}
		pr.ReviewRequests = append(pr.ReviewRequests, struct {
			Login string `json:"login"`
		}{Login: login})
	}

	if node.StatusCheckRollup != nil && strings.TrimSpace(node.StatusCheckRollup.State) != "" {
		pr.StatusCheckRollup = append(pr.StatusCheckRollup, struct {
			State string `json:"state"`
		}{State: node.StatusCheckRollup.State})
	}

	if includeCommits && node.Commits != nil {
		for _, c := range node.Commits.Nodes {
			if strings.TrimSpace(c.Commit.OID) == "" {
				continue
			}
			pr.Commits = append(pr.Commits, PullRequestCommit{OID: c.Commit.OID})
		}
	}

	return pr
}

func normalizeReviewers(reviewers []string) []string {
	if len(reviewers) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(reviewers))
	result := make([]string, 0, len(reviewers))
	for _, reviewer := range reviewers {
		reviewer = strings.TrimSpace(reviewer)
		if reviewer == "" {
			continue
		}
		if _, ok := seen[reviewer]; ok {
			continue
		}
		seen[reviewer] = struct{}{}
		result = append(result, reviewer)
	}
	return result
}
