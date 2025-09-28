package cmd

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/pmenglund/stacky/internal/config"
	"github.com/pmenglund/stacky/internal/engine"
)

type fakeImportEngine struct {
	cfg       config.Config
	plan      engine.ImportPlan
	planErr   error
	execErr   error
	executed  []engine.ImportPlan
	requested []string
}

func (f *fakeImportEngine) Config() config.Config {
	return f.cfg
}

func (f *fakeImportEngine) PlanImport(_ context.Context, branch string) (engine.ImportPlan, error) {
	f.requested = append(f.requested, branch)
	if f.planErr != nil {
		return engine.ImportPlan{}, f.planErr
	}
	return f.plan, nil
}

func (f *fakeImportEngine) ExecuteImportPlan(_ context.Context, plan engine.ImportPlan) error {
	f.executed = append(f.executed, plan)
	return f.execErr
}

func TestRunImportExecutesPlanAfterConfirmation(t *testing.T) {
	eng := &fakeImportEngine{plan: engine.ImportPlan{
		TopBranch:  "topic2",
		BaseBranch: "main",
		Lookups:    []string{"topic2", "topic"},
		Actions:    []engine.ImportAction{{Branch: "topic", Parent: "main", ParentCommit: "abc"}, {Branch: "topic2", Parent: "topic", ParentCommit: "def"}},
	}}

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	out := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(errBuf)
	cmd.SetIn(strings.NewReader("yes\n"))

	require.NoError(t, runImport(cmd, eng, "topic2", false))
	require.Equal(t, []string{"topic2"}, eng.requested)
	require.Len(t, eng.executed, 1)
	require.Contains(t, errBuf.String(), "Getting PR information for topic2")
	require.Contains(t, out.String(), "Will set parent of topic2 to topic")
}

func TestRunImportSkipsConfirmationWhenForced(t *testing.T) {
	eng := &fakeImportEngine{plan: engine.ImportPlan{
		Actions: []engine.ImportAction{{Branch: "topic", Parent: "main", ParentCommit: "abc"}},
	}}

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(&bytes.Buffer{})

	require.NoError(t, runImport(cmd, eng, "topic", true))
	require.Len(t, eng.executed, 1)
	require.NotContains(t, out.String(), "Proceed? [yes/no]")
}

func TestRunImportSkipsConfirmationWhenConfigured(t *testing.T) {
	eng := &fakeImportEngine{
		cfg:  config.Config{SkipConfirm: true},
		plan: engine.ImportPlan{Actions: []engine.ImportAction{{Branch: "topic", Parent: "main", ParentCommit: "abc"}}},
	}

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	require.NoError(t, runImport(cmd, eng, "topic", false))
	require.Len(t, eng.executed, 1)
}

func TestRunImportNoActionsReturnsEarly(t *testing.T) {
	eng := &fakeImportEngine{plan: engine.ImportPlan{TopBranch: "topic", BaseBranch: "main"}}

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(&bytes.Buffer{})

	require.NoError(t, runImport(cmd, eng, "topic", false))
	require.Empty(t, eng.executed)
	require.Contains(t, out.String(), "nothing to import")
}

func TestRunImportPropagatesErrors(t *testing.T) {
	expected := errors.New("boom")
	eng := &fakeImportEngine{planErr: expected}

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := runImport(cmd, eng, "topic", false)
	require.ErrorIs(t, err, expected)
}

func TestRenderImportPlanFormatsActions(t *testing.T) {
	plan := engine.ImportPlan{
		Actions: []engine.ImportAction{{Branch: "topic", Parent: "main", ParentCommit: "abc"}},
	}
	out := renderImportPlan(plan)
	require.Contains(t, out, "Will set parent of topic to main")
}
