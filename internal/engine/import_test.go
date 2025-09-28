package engine_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pmenglund/stacky/internal/engine"
	"github.com/pmenglund/stacky/internal/githubcli"
)

func TestPlanImportAndExecute(t *testing.T) {
	ctx := context.Background()
	repoDir := initRepo(t)

	runGit(t, repoDir, "checkout", "main")
	runGit(t, repoDir, "checkout", "-b", "topic")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "topic.txt"), []byte("topic"), 0o644))
	runGit(t, repoDir, "add", "topic.txt")
	runGit(t, repoDir, "commit", "-m", "topic commit")

	runGit(t, repoDir, "checkout", "-b", "topic2")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "topic2.txt"), []byte("topic2"), 0o644))
	runGit(t, repoDir, "add", "topic2.txt")
	runGit(t, repoDir, "commit", "-m", "topic2 commit")

	runGit(t, repoDir, "checkout", "topic")

	topicFirst := firstCommitOnBranch(t, repoDir, "main", "topic")
	topic2First := firstCommitOnBranch(t, repoDir, "topic", "topic2")

	gh := &fakeGitHub{headPRs: map[string][]githubcli.PullRequest{
		"topic": {
			{
				Number:  11,
				State:   "OPEN",
				HeadRef: "topic",
				BaseRef: "main",
				Commits: []githubcli.PullRequestCommit{{OID: topicFirst}},
			},
		},
		"topic2": {
			{
				Number:  12,
				State:   "OPEN",
				HeadRef: "topic2",
				BaseRef: "topic",
				Commits: []githubcli.PullRequestCommit{{OID: topic2First}},
			},
		},
	}}

	eng, err := engine.New(ctx, engine.Options{RepoPath: repoDir, HomeDir: t.TempDir(), GitHub: gh})
	require.NoError(t, err)

	plan, err := eng.PlanImport(ctx, "topic2")
	require.NoError(t, err)
	require.Equal(t, "topic2", plan.TopBranch)
	require.Equal(t, "main", plan.BaseBranch)
	require.Equal(t, []string{"topic2", "topic"}, plan.Lookups)
	require.Len(t, plan.Actions, 2)

	require.Equal(t, engine.ImportAction{Branch: "topic", Parent: "main", ParentCommit: parentCommitFor(t, repoDir, topicFirst)}, plan.Actions[0])
	require.Equal(t, engine.ImportAction{Branch: "topic2", Parent: "topic", ParentCommit: parentCommitFor(t, repoDir, topic2First)}, plan.Actions[1])

	require.Len(t, gh.params, 2)
	require.Equal(t, "all", gh.params[0].State)
	require.Contains(t, gh.params[0].Fields, "commits")
	require.Equal(t, "topic2", gh.params[0].Head)
	require.Equal(t, "topic", gh.params[1].Head)

	require.NoError(t, eng.ExecuteImportPlan(ctx, plan))

	topicRemote := strings.TrimSpace(runGitOutput(t, repoDir, "config", "branch.topic.remote"))
	topicMerge := strings.TrimSpace(runGitOutput(t, repoDir, "config", "branch.topic.merge"))
	topicParent := strings.TrimSpace(runGitOutput(t, repoDir, "rev-parse", "refs/stack-parent/topic"))

	require.Equal(t, ".", topicRemote)
	require.Equal(t, "refs/heads/main", topicMerge)
	require.Equal(t, parentCommitFor(t, repoDir, topicFirst), topicParent)

	topic2Remote := strings.TrimSpace(runGitOutput(t, repoDir, "config", "branch.topic2.remote"))
	topic2Merge := strings.TrimSpace(runGitOutput(t, repoDir, "config", "branch.topic2.merge"))
	topic2Parent := strings.TrimSpace(runGitOutput(t, repoDir, "rev-parse", "refs/stack-parent/topic2"))

	require.Equal(t, ".", topic2Remote)
	require.Equal(t, "refs/heads/topic", topic2Merge)
	require.Equal(t, parentCommitFor(t, repoDir, topic2First), topic2Parent)
}

func TestPlanImportNoOpenPR(t *testing.T) {
	ctx := context.Background()
	repoDir := initRepo(t)
	runGit(t, repoDir, "checkout", "main")
	runGit(t, repoDir, "checkout", "-b", "topic")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "topic.txt"), []byte("topic"), 0o644))
	runGit(t, repoDir, "add", "topic.txt")
	runGit(t, repoDir, "commit", "-m", "topic commit")

	topicFirst := firstCommitOnBranch(t, repoDir, "main", "topic")

	gh := &fakeGitHub{headPRs: map[string][]githubcli.PullRequest{
		"topic": {
			{
				Number:  21,
				State:   "CLOSED",
				HeadRef: "topic",
				BaseRef: "main",
				Commits: []githubcli.PullRequestCommit{{OID: topicFirst}},
			},
		},
	}}

	eng, err := engine.New(ctx, engine.Options{RepoPath: repoDir, HomeDir: t.TempDir(), GitHub: gh})
	require.NoError(t, err)

	_, err = eng.PlanImport(ctx, "topic")
	require.Error(t, err)
	require.Contains(t, err.Error(), "no open pull request")
}

func firstCommitOnBranch(t *testing.T, dir, base, branch string) string {
	t.Helper()
	revs := strings.TrimSpace(runGitOutput(t, dir, "rev-list", fmt.Sprintf("%s..%s", base, branch), "--reverse"))
	parts := strings.Split(revs, "\n")
	require.NotEmpty(t, parts)
	return strings.TrimSpace(parts[0])
}

func parentCommitFor(t *testing.T, dir, commit string) string {
	t.Helper()
	out := strings.TrimSpace(runGitOutput(t, dir, "rev-parse", fmt.Sprintf("%s^", commit)))
	require.NotEmpty(t, out)
	return out
}
