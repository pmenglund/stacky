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

func TestLandMergesPullRequest(t *testing.T) {
	repoDir := initGitRepo(t)
	homeDir := t.TempDir()

	remoteBase := t.TempDir()
	remoteDir := filepath.Join(remoteBase, "remote.git")
	runGit(t, remoteBase, "init", "--bare", remoteDir)
	runGit(t, repoDir, "remote", "add", "origin", remoteDir)
	runGit(t, repoDir, "push", "origin", "main")

	stdout, stderr, err := runStacky(t, repoDir, homeDir, nil, "branch", "new", "feature")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Empty(t, stdout)

	featureFile := filepath.Join(repoDir, "feature.txt")
	require.NoError(t, os.WriteFile(featureFile, []byte("feature\n"), 0o644))
	runGit(t, repoDir, "add", "feature.txt")
	runGit(t, repoDir, "commit", "-m", "feature change")
	runGit(t, repoDir, "push", "origin", "feature")

	stdout, stderr, err = runStacky(t, repoDir, homeDir, nil, "branch", "new", "feature-child")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Empty(t, stdout)

	childFile := filepath.Join(repoDir, "child.txt")
	require.NoError(t, os.WriteFile(childFile, []byte("child\n"), 0o644))
	runGit(t, repoDir, "add", "child.txt")
	runGit(t, repoDir, "commit", "-m", "child change")

	ghDir := t.TempDir()
	logPath := filepath.Join(ghDir, "gh.log")
	script := fmt.Sprintf(`#!/bin/sh
set -e
printf "%%s\n" "$*" >> "%s"
if [ "$1" = "pr" ] && [ "$2" = "list" ]; then
  cat <<'JSON'
[{"number":17,"url":"https://example.com/pr/17","headRefName":"feature","mergeable":"MERGEABLE"}]
JSON
  exit 0
fi
if [ "$1" = "pr" ] && [ "$2" = "merge" ]; then
  exit 0
fi
echo "unexpected gh invocation: $*" >&2
exit 1
`, logPath)
	ghPath := filepath.Join(ghDir, "gh")
	require.NoError(t, os.WriteFile(ghPath, []byte(script), 0o755))

	prevPath := os.Getenv("PATH")
	newPath := ghDir
	if prevPath != "" {
		newPath = ghDir + string(os.PathListSeparator) + prevPath
	}
	require.NoError(t, os.Setenv("PATH", newPath))
	defer func() {
		if prevPath == "" {
			require.NoError(t, os.Unsetenv("PATH"))
			return
		}
		require.NoError(t, os.Setenv("PATH", prevPath))
	}()

	stdin := bytes.NewReader([]byte("yes\n"))
	stdout, stderr, err = runStacky(t, repoDir, homeDir, stdin, "--color=never", "land")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Will land PR #17")
	require.Contains(t, stdout, "Success! Run `stacky update`")

	data, err := os.ReadFile(logPath)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.NotEmpty(t, lines)

	sawList := false
	sawMerge := false
	for _, line := range lines {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 {
			continue
		}
		if fields[0] == "pr" && fields[1] == "list" {
			sawList = true
			continue
		}
		if fields[0] == "pr" && fields[1] == "merge" {
			sawMerge = true
			require.GreaterOrEqual(t, len(fields), 5)
			require.Equal(t, "feature", fields[2])
			require.Contains(t, fields, "--match-head-commit")
			for _, field := range fields {
				require.NotEqual(t, "--auto", field)
			}
		}
	}

	require.True(t, sawList)
	require.True(t, sawMerge)
}
