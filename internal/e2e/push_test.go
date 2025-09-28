package e2e_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStackPushPushesBranches(t *testing.T) {
	repoDir := initGitRepo(t)
	homeDir := t.TempDir()

	remoteBase := t.TempDir()
	remoteDir := filepath.Join(remoteBase, "remote.git")
	runGit(t, remoteBase, "init", "--bare", remoteDir)
	runGit(t, repoDir, "remote", "add", "origin", remoteDir)

	stdout, stderr, err := runStacky(t, repoDir, homeDir, nil, "branch", "new", "feature")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Empty(t, stdout)

	featureFile := filepath.Join(repoDir, "feature.txt")
	require.NoError(t, os.WriteFile(featureFile, []byte("feature\n"), 0o644))
	runGit(t, repoDir, "add", "feature.txt")
	runGit(t, repoDir, "commit", "-m", "feature change")

	stdout, stderr, err = runStacky(t, repoDir, homeDir, nil, "branch", "new", "feature-child")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Empty(t, stdout)

	childFile := filepath.Join(repoDir, "child.txt")
	require.NoError(t, os.WriteFile(childFile, []byte("child\n"), 0o644))
	runGit(t, repoDir, "add", "child.txt")
	runGit(t, repoDir, "commit", "-m", "child change")

	stdout, stderr, err = runStacky(t, repoDir, homeDir, nil, "--color=never", "stack", "push", "--force", "--no-pr")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Will push branch feature to origin/feature")
	require.Contains(t, stdout, "Will push branch feature-child to origin/feature-child")

	remoteFeature := strings.TrimSpace(runGit(t, remoteDir, "rev-parse", "refs/heads/feature"))
	localFeature := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "refs/heads/feature"))
	require.Equal(t, localFeature, remoteFeature)

	remoteChild := strings.TrimSpace(runGit(t, remoteDir, "rev-parse", "refs/heads/feature-child"))
	localChild := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "refs/heads/feature-child"))
	require.Equal(t, localChild, remoteChild)
}

func TestStackPushCreatesPullRequests(t *testing.T) {
	repoDir := initGitRepo(t)
	homeDir := t.TempDir()

	remoteBase := t.TempDir()
	remoteDir := filepath.Join(remoteBase, "remote.git")
	runGit(t, remoteBase, "init", "--bare", remoteDir)
	runGit(t, repoDir, "remote", "add", "origin", remoteDir)

	stdout, stderr, err := runStacky(t, repoDir, homeDir, nil, "branch", "new", "feature")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Empty(t, stdout)

	featureFile := filepath.Join(repoDir, "feature.txt")
	require.NoError(t, os.WriteFile(featureFile, []byte("feature\n"), 0o644))
	runGit(t, repoDir, "add", "feature.txt")
	runGit(t, repoDir, "commit", "-m", "feature change")

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
  echo "[]"
fi
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

	stdout, stderr, err = runStacky(t, repoDir, homeDir, nil, "--color=never", "stack", "push", "--force")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Will create PR for branch feature")
	require.Contains(t, stdout, "Will create PR for branch feature-child")

	data, err := os.ReadFile(logPath)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.NotEmpty(t, lines)

	var created []string
	var listCount int
	for _, line := range lines {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 {
			continue
		}
		if fields[0] == "pr" && fields[1] == "list" {
			listCount++
			continue
		}
		if fields[0] == "pr" && fields[1] == "create" {
			for i := 0; i < len(fields); i++ {
				if fields[i] == "--head" && i+1 < len(fields) {
					created = append(created, fields[i+1])
				}
			}
		}
	}

	require.GreaterOrEqual(t, listCount, 2)
	require.ElementsMatch(t, []string{"feature", "feature-child"}, created)
}
