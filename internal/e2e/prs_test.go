package e2e_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPrsAllowsEditingPullRequest(t *testing.T) {
	repoDir := initGitRepo(t)
	homeDir := t.TempDir()

	ghDir := t.TempDir()
	logPath := filepath.Join(ghDir, "gh.log")

	script := fmt.Sprintf(`#!/bin/sh
set -eu
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
    "title": "Review teammate",
    "baseRefName": "main",
    "headRefName": "review-branch",
    "mergeable": "MERGEABLE",
    "mergeStateStatus": "CLEAN",
    "updatedAt": "2025-10-15T16:00:00Z",
    "createdAt": "2025-10-14T15:00:00Z",
    "reviewDecision": "REVIEW_REQUIRED",
    "isDraft": false,
    "author": { "login": "coworker" },
    "reviewRequests": [],
    "statusCheckRollup": [],
    "body": "Needs review"
  }
]
JSON
  else
    cat <<'JSON'
[
  {
    "id": "PR_101",
    "number": 101,
    "state": "OPEN",
    "url": "https://example.com/pr/101",
    "title": "Feature branch",
    "baseRefName": "main",
    "headRefName": "feature-branch",
    "mergeable": "MERGEABLE",
    "mergeStateStatus": "CLEAN",
    "updatedAt": "2025-10-15T12:00:00Z",
    "createdAt": "2025-10-14T09:00:00Z",
    "reviewDecision": "REVIEW_REQUIRED",
    "isDraft": false,
    "author": { "login": "me" },
    "reviewRequests": [],
    "statusCheckRollup": [],
    "body": "Existing body"
  },
  {
    "id": "PR_102",
    "number": 102,
    "state": "OPEN",
    "url": "https://example.com/pr/102",
    "title": "Refactor helper",
    "baseRefName": "main",
    "headRefName": "refactor-helper",
    "mergeable": "MERGEABLE",
    "mergeStateStatus": "CLEAN",
    "updatedAt": "2025-10-15T11:00:00Z",
    "createdAt": "2025-10-13T08:00:00Z",
    "reviewDecision": "REVIEW_REQUIRED",
    "isDraft": false,
    "author": { "login": "me" },
    "reviewRequests": [],
    "statusCheckRollup": [],
    "body": "Second body"
  }
]
JSON
  fi
  exit 0
fi
if [ "$1" = "pr" ] && [ "$2" = "edit" ]; then
  exit 0
fi
echo "unexpected gh invocation: $@" >&2
exit 1
`, logPath)

	ghPath := filepath.Join(ghDir, "gh")
	require.NoError(t, os.WriteFile(ghPath, []byte(script), 0o755))

	editorPath := filepath.Join(ghDir, "editor")
	editorScript := "#!/bin/sh\nset -eu\nprintf 'Updated PR body\n' > \"$1\"\n"
	require.NoError(t, os.WriteFile(editorPath, []byte(editorScript), 0o755))

	pathEnv := ghDir
	if existing := os.Getenv("PATH"); existing != "" {
		pathEnv = ghDir + string(os.PathListSeparator) + existing
	}
	t.Setenv("PATH", pathEnv)
	t.Setenv("EDITOR", editorPath)

	stdin := bytes.NewReader([]byte("1\nq\n"))
	stdout, stderr, err := runStacky(t, repoDir, homeDir, stdin, "--color=never", "prs")
	require.NoError(t, err)
	require.Empty(t, stderr)

	require.Contains(t, stdout, "Select a pull request to edit its description:")
	require.Contains(t, stdout, "#101 Feature branch")
	require.Contains(t, stdout, "#102 Refactor helper")
	require.Contains(t, stdout, "#201 Review teammate")
	require.Contains(t, stdout, "Editing PR #101 - Feature branch")
	require.Contains(t, stdout, "Current description:")
	require.Contains(t, stdout, "Existing body")
	require.Contains(t, stdout, "Updating PR description...")
	require.Contains(t, stdout, "✓ Successfully updated PR #101 description")

	data, err := os.ReadFile(logPath)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.NotEmpty(t, lines)

	var authoredCall, reviewCall, editCall bool
	for _, line := range lines {
		if strings.Contains(line, "pr list") && strings.Contains(line, "--author") {
			authoredCall = true
		}
		if strings.Contains(line, "pr list") && strings.Contains(line, "review-requested:@me") {
			reviewCall = true
		}
		if strings.Contains(line, "pr edit") {
			if strings.Contains(line, "--body Updated PR body") {
				editCall = true
			}
		}
	}

	require.True(t, authoredCall, "expected gh to be called with --author @me")
	require.True(t, reviewCall, "expected gh to be called with review-requested:@me")
	require.True(t, editCall, "expected gh to update PR body")
}
