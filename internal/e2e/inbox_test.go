package e2e_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInboxRendersAuthoredAndReviewSections(t *testing.T) {
	repoDir := initGitRepo(t)
	homeDir := t.TempDir()

	ghDir := t.TempDir()
	logPath := filepath.Join(ghDir, "gh.log")

	script := fmt.Sprintf(`#!/bin/sh
printf "%%s\n" "$*" >> "%s"
if [ "$1" = "pr" ] && [ "$2" = "list" ]; then
  args="$*"
  if printf "%%s" "$args" | grep -q "review-requested:@me"; then
    cat <<'JSON'
[
  {
    "id": "PR_201",
    "number": 201,
    "state": "OPEN",
    "url": "https://example.com/pr/201",
    "title": "Needs review",
    "baseRefName": "main",
    "headRefName": "friend-branch",
    "mergeable": "MERGEABLE",
    "mergeStateStatus": "CLEAN",
    "updatedAt": "2025-10-13T15:00:00Z",
    "createdAt": "2025-10-12T14:00:00Z",
    "reviewDecision": "REVIEW_REQUIRED",
    "isDraft": false,
    "author": { "login": "friend" },
    "reviewRequests": [],
    "statusCheckRollup": [
      { "state": "SUCCESS" }
    ],
    "body": "",
    "commits": []
  },
  {
    "id": "PR_202",
    "number": 202,
    "state": "OPEN",
    "url": "https://example.com/pr/202",
    "title": "Draft review",
    "baseRefName": "main",
    "headRefName": "draft-branch",
    "mergeable": "MERGEABLE",
    "mergeStateStatus": "CLEAN",
    "updatedAt": "2025-10-14T16:00:00Z",
    "createdAt": "2025-10-13T13:00:00Z",
    "reviewDecision": "REVIEW_REQUIRED",
    "isDraft": true,
    "author": { "login": "friend2" },
    "reviewRequests": [],
    "statusCheckRollup": [
      { "state": "FAILURE" }
    ],
    "body": "",
    "commits": []
  }
]
JSON
  else
    cat <<'JSON'
[
  {
    "id": "PR_12",
    "number": 12,
    "state": "OPEN",
    "url": "https://example.com/pr/12",
    "title": "Draft feature",
    "baseRefName": "main",
    "headRefName": "draft-feature",
    "mergeable": "MERGEABLE",
    "mergeStateStatus": "CLEAN",
    "updatedAt": "2025-10-14T12:00:00Z",
    "createdAt": "2025-10-10T09:00:00Z",
    "reviewDecision": "REVIEW_REQUIRED",
    "isDraft": true,
    "author": { "login": "me" },
    "reviewRequests": [],
    "statusCheckRollup": [
      { "state": "QUEUED" }
    ],
    "body": "",
    "commits": []
  },
  {
    "id": "PR_13",
    "number": 13,
    "state": "OPEN",
    "url": "https://example.com/pr/13",
    "title": "Awaiting review",
    "baseRefName": "main",
    "headRefName": "feature-awaiting-review",
    "mergeable": "MERGEABLE",
    "mergeStateStatus": "CLEAN",
    "updatedAt": "2025-10-13T11:00:00Z",
    "createdAt": "2025-10-09T08:00:00Z",
    "reviewDecision": "REVIEW_REQUIRED",
    "isDraft": false,
    "author": { "login": "me" },
    "reviewRequests": [
      { "login": "teammate" }
    ],
    "statusCheckRollup": [
      { "state": "SUCCESS" }
    ],
    "body": "",
    "commits": []
  },
  {
    "id": "PR_14",
    "number": 14,
    "state": "OPEN",
    "url": "https://example.com/pr/14",
    "title": "Approved branch",
    "baseRefName": "main",
    "headRefName": "feature-approved",
    "mergeable": "MERGEABLE",
    "mergeStateStatus": "CLEAN",
    "updatedAt": "2025-10-12T10:00:00Z",
    "createdAt": "2025-10-08T07:00:00Z",
    "reviewDecision": "APPROVED",
    "isDraft": false,
    "author": { "login": "me" },
    "reviewRequests": [],
    "statusCheckRollup": [],
    "body": "",
    "commits": []
  }
]
JSON
  fi
  exit 0
fi
echo "unexpected gh invocation: $@" >&2
exit 1
`, logPath)

	ghPath := filepath.Join(ghDir, "gh")
	require.NoError(t, os.WriteFile(ghPath, []byte(script), 0o755))

	pathEnv := ghDir
	if existing := os.Getenv("PATH"); existing != "" {
		pathEnv = ghDir + string(os.PathListSeparator) + existing
	}
	t.Setenv("PATH", pathEnv)

	stdout, stderr, err := runStacky(t, repoDir, homeDir, nil, "--color=never", "inbox", "--compact")
	require.NoError(t, err)
	require.Empty(t, stderr)

	require.Contains(t, stdout, "Your PRs - Waiting on You:\n")
	require.Contains(t, stdout, "#12 Draft feature (draft-feature) [DRAFT] ⏳ Checks running Updated: 2025-10-14\n")
	require.Contains(t, stdout, "Your PRs - Waiting on Review:\n")
	require.Contains(t, stdout, "#13 Awaiting review (feature-awaiting-review) ✓ Checks passed Updated: 2025-10-13\n")
	require.Contains(t, stdout, "Your PRs - Approved:\n")
	require.Contains(t, stdout, "#14 Approved branch (feature-approved) Updated: 2025-10-12\n")
	require.Contains(t, stdout, "Pull Requests Awaiting Your Review:\n")
	require.Contains(t, stdout, "#201 Needs review (friend-branch) by friend ✓ Checks passed Updated: 2025-10-13\n")
	require.Contains(t, stdout, "#202 Draft review (draft-branch) by friend2 [DRAFT] ✗ Checks failed Updated: 2025-10-14\n")

	data, err := os.ReadFile(logPath)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.NotEmpty(t, lines)

	var authoredCall, reviewCall bool
	for _, line := range lines {
		if strings.Contains(line, "--author") {
			authoredCall = true
		}
		if strings.Contains(line, "review-requested:@me") {
			reviewCall = true
		}
	}
	require.True(t, authoredCall, "expected gh to be called with --author @me")
	require.True(t, reviewCall, "expected gh to be called with review-requested:@me search")
}
