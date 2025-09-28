package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/pmenglund/stacky/internal/config"
	"github.com/pmenglund/stacky/internal/engine"
)

type stubUpdateEngine struct {
	cfg       config.Config
	plan      engine.UpdatePlan
	planErr   error
	execErr   error
	gotRemote string
	executed  []engine.UpdatePlan
}

func (s *stubUpdateEngine) Config() config.Config { return s.cfg }

func (s *stubUpdateEngine) PlanUpdate(_ context.Context, remote string) (engine.UpdatePlan, error) {
	s.gotRemote = remote
	if s.planErr != nil {
		return engine.UpdatePlan{}, s.planErr
	}
	return s.plan, nil
}

func (s *stubUpdateEngine) ExecuteUpdatePlan(_ context.Context, plan engine.UpdatePlan) error {
	s.executed = append(s.executed, plan)
	return s.execErr
}

func TestRunUpdateFlowExecutesPlanAfterConfirmation(t *testing.T) {
	stub := &stubUpdateEngine{plan: engine.UpdatePlan{
		Remote:    "origin",
		Bottoms:   []engine.UpdateBottom{{Branch: "main", NeedsUpdate: true}},
		Deletions: []engine.UpdateDeletion{{Branch: "feature", Parent: "main", PRNumber: 42, Children: []string{"child"}}},
	}}

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetIn(strings.NewReader("yes\n"))

	require.NoError(t, runUpdateFlow(cmd, stub, "origin", false))
	require.Equal(t, "origin", stub.gotRemote)
	require.Len(t, stub.executed, 1)
	require.Contains(t, stdout.String(), "Will delete branch feature")
	require.Contains(t, stdout.String(), "Proceed? [yes/no]")
}

func TestRunUpdateFlowSkipsConfirmationWhenNoDeletions(t *testing.T) {
	stub := &stubUpdateEngine{plan: engine.UpdatePlan{
		Remote:  "origin",
		Bottoms: []engine.UpdateBottom{{Branch: "main", NeedsUpdate: true}},
	}}

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})

	require.NoError(t, runUpdateFlow(cmd, stub, "origin", false))
	require.Len(t, stub.executed, 1)
	require.NotContains(t, stdout.String(), "Proceed? [yes/no]")
}

func TestRunUpdateFlowSkipsConfirmationWhenForced(t *testing.T) {
	stub := &stubUpdateEngine{plan: engine.UpdatePlan{
		Remote:    "origin",
		Bottoms:   []engine.UpdateBottom{{Branch: "main"}},
		Deletions: []engine.UpdateDeletion{{Branch: "feature", Parent: "main", PRNumber: 7}},
	}}

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})

	require.NoError(t, runUpdateFlow(cmd, stub, "origin", true))
	require.Len(t, stub.executed, 1)
	require.NotContains(t, stdout.String(), "Proceed? [yes/no]")
}

func TestRunUpdateFlowSkipsWhenNoWork(t *testing.T) {
	stub := &stubUpdateEngine{plan: engine.UpdatePlan{
		Remote: "origin",
	}}

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	require.NoError(t, runUpdateFlow(cmd, stub, "origin", false))
	require.Empty(t, stub.executed)
}

func TestRenderUpdatePlanIncludesSummary(t *testing.T) {
	plan := engine.UpdatePlan{
		Remote: "origin",
		Bottoms: []engine.UpdateBottom{
			{Branch: "main", NeedsUpdate: true},
			{Branch: "trunk", NeedsUpdate: false},
		},
		Deletions: []engine.UpdateDeletion{{Branch: "feature", Parent: "main", PRNumber: 9, Children: []string{"child"}}},
	}

	out := renderUpdatePlan(plan)
	require.Contains(t, out, "fast-forward bottom branch main")
	require.Contains(t, out, "already matches origin/trunk")
	require.Contains(t, out, "Will delete branch feature")
	require.Contains(t, out, "Will reparent branch child onto main")
}
