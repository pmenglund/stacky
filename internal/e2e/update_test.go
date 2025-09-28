package e2e_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStackUpdateDeletesMergedBranch(t *testing.T) {
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

	stdout, stderr, err = runStacky(t, repoDir, homeDir, nil, "branch", "new", "feature-child")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Empty(t, stdout)

	childFile := filepath.Join(repoDir, "child.txt")
	require.NoError(t, os.WriteFile(childFile, []byte("child\n"), 0o644))
	runGit(t, repoDir, "add", "child.txt")
	runGit(t, repoDir, "commit", "-m", "child change")

	runGit(t, repoDir, "checkout", "main")

	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "remote.txt"), []byte("remote\n"), 0o644))
	runGit(t, repoDir, "add", "remote.txt")
	runGit(t, repoDir, "commit", "-m", "remote change")
	runGit(t, repoDir, "push", "origin", "main")

	remoteTip := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	runGit(t, repoDir, "reset", "--hard", "HEAD^")

	ghDir := t.TempDir()
	script := "#!/bin/sh\nset -eu\nif [ \"$1\" = \"pr\" ] && [ \"$2\" = \"list\" ]; then\n  head=\"\"\n  while [ $# -gt 0 ]; do\n    case \"$1\" in\n      --head)\n        shift\n        head=\"$1\"\n        ;;\n    esac\n    shift\n  done\n  if [ \"$head\" = \"feature\" ]; then\n    printf '[{\"number\":42,\"state\":\"MERGED\",\"headRefName\":\"feature\",\"baseRefName\":\"main\"}]\n'\n    exit 0\n  fi\n  if [ \"$head\" = \"feature-child\" ]; then\n    printf '[{\"number\":43,\"state\":\"OPEN\",\"headRefName\":\"feature-child\",\"baseRefName\":\"feature\"}]\n'\n    exit 0\n  fi\n  printf '[]\n'\n  exit 0\nfi\nprintf ''\n"
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

	stdout, stderr, err = runStacky(t, repoDir, homeDir, nil, "--color=never", "update", "--force")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Will fast-forward bottom branch main to origin/main")
	require.Contains(t, stdout, "Will delete branch feature; PR #42 merged into main")
	require.Contains(t, stdout, "Will reparent branch feature-child onto main")

	branches := strings.TrimSpace(runGit(t, repoDir, "branch", "--list", "feature"))
	require.Equal(t, "", branches)

	mergeTarget := strings.TrimSpace(runGit(t, repoDir, "config", "branch.feature-child.merge"))
	require.Equal(t, "refs/heads/main", mergeTarget)

	parentRef := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "refs/stack-parent/feature-child"))
	require.Equal(t, remoteTip, parentRef)

	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/stack-parent/feature")
	cmd.Dir = repoDir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	require.Error(t, cmd.Run())

	localMain := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "main"))
	require.Equal(t, remoteTip, localMain)
}
