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

func TestImportSetsParentRefsAndConfig(t *testing.T) {
	repoDir := initGitRepo(t)
	homeDir := t.TempDir()

	runGit(t, repoDir, "checkout", "-b", "feature")
	featureFile := filepath.Join(repoDir, "feature.txt")
	require.NoError(t, os.WriteFile(featureFile, []byte("feature\n"), 0o644))
	runGit(t, repoDir, "add", "feature.txt")
	runGit(t, repoDir, "commit", "-m", "feature change")

	featureCommit := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "feature"))
	featureParent := strings.TrimSpace(runGit(t, repoDir, "rev-parse", fmt.Sprintf("%s^", featureCommit)))

	runGit(t, repoDir, "checkout", "-b", "feature-top")
	topFile := filepath.Join(repoDir, "feature-top.txt")
	require.NoError(t, os.WriteFile(topFile, []byte("feature top\n"), 0o644))
	runGit(t, repoDir, "add", "feature-top.txt")
	runGit(t, repoDir, "commit", "-m", "feature top change")

	topCommit := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "feature-top"))
	topParent := strings.TrimSpace(runGit(t, repoDir, "rev-parse", fmt.Sprintf("%s^", topCommit)))

	runGit(t, repoDir, "checkout", "main")

	ghDir := t.TempDir()
	logPath := filepath.Join(ghDir, "gh.log")

	topJSON := fmt.Sprintf(`[
  {
    "id": "PR_feature_top",
    "number": 202,
    "state": "OPEN",
    "baseRefName": "feature",
    "headRefName": "feature-top",
    "commits": [
      {"oid": "%s"}
    ]
  }
]`, topCommit)

	featureJSON := fmt.Sprintf(`[
  {
    "id": "PR_feature",
    "number": 201,
    "state": "OPEN",
    "baseRefName": "main",
    "headRefName": "feature",
    "commits": [
      {"oid": "%s"}
    ]
  }
]`, featureCommit)

	script := fmt.Sprintf(`#!/bin/sh
set -eu
printf "%%s\n" "$*" >> "%s"
if [ "$1" = "pr" ] && [ "$2" = "list" ]; then
  case "$*" in
    *"--head feature-top"*)
      cat <<'JSON'
%s
JSON
      exit 0
      ;;
    *"--head feature"*)
      cat <<'JSON'
%s
JSON
      exit 0
      ;;
  esac
fi
echo "unexpected gh invocation: $@" >&2
exit 1
`, logPath, topJSON, featureJSON)

	ghPath := filepath.Join(ghDir, "gh")
	require.NoError(t, os.WriteFile(ghPath, []byte(script), 0o755))

	pathEnv := ghDir
	if existing := os.Getenv("PATH"); existing != "" {
		pathEnv = ghDir + string(os.PathListSeparator) + existing
	}
	t.Setenv("PATH", pathEnv)

	stdin := bytes.NewReader([]byte("yes\n"))
	stdout, stderr, err := runStacky(t, repoDir, homeDir, stdin, "--color=never", "import", "feature-top")
	require.NoError(t, err)

	require.Contains(t, stderr, "Getting PR information for feature-top")
	require.Contains(t, stderr, "Getting PR information for feature")
	require.Contains(t, stdout, fmt.Sprintf("- Will set parent of feature to main at commit %s\n", featureParent))
	require.Contains(t, stdout, fmt.Sprintf("- Will set parent of feature-top to feature at commit %s\n", topParent))

	featureParentRef := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "refs/stack-parent/feature"))
	require.Equal(t, featureParent, featureParentRef)

	topParentRef := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "refs/stack-parent/feature-top"))
	require.Equal(t, topParent, topParentRef)

	featureMerge := strings.TrimSpace(runGit(t, repoDir, "config", "branch.feature.merge"))
	require.Equal(t, "refs/heads/main", featureMerge)
	featureRemote := strings.TrimSpace(runGit(t, repoDir, "config", "branch.feature.remote"))
	require.Equal(t, ".", featureRemote)

	topMerge := strings.TrimSpace(runGit(t, repoDir, "config", "branch.feature-top.merge"))
	require.Equal(t, "refs/heads/feature", topMerge)
	topRemote := strings.TrimSpace(runGit(t, repoDir, "config", "branch.feature-top.remote"))
	require.Equal(t, ".", topRemote)

	data, err := os.ReadFile(logPath)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.GreaterOrEqual(t, len(lines), 2)

	sawFeature := false
	sawTop := false
	for _, line := range lines {
		if strings.Contains(line, "--head feature-top") {
			sawTop = true
		}
		if strings.Contains(line, "--head feature") {
			sawFeature = true
		}
	}

	require.True(t, sawTop, "expected gh to be invoked for feature-top")
	require.True(t, sawFeature, "expected gh to be invoked for feature")
}
