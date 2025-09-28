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

type stubPushEngine struct {
	cfg      config.Config
	executed []engine.PushPlan
	execErr  error
}

func (s *stubPushEngine) Config() config.Config {
	return s.cfg
}

func (s *stubPushEngine) ExecutePushPlan(_ context.Context, plan engine.PushPlan) error {
	s.executed = append(s.executed, plan)
	return s.execErr
}

func TestRunPushFlowExecutesPlanAfterConfirmation(t *testing.T) {
	eng := &stubPushEngine{}
	plan := engine.PushPlan{
		Remote: "origin",
		Actions: []engine.PushAction{
			{Branch: "main", IsBase: true},
			{Branch: "feature", Parent: "main", Push: true},
		},
	}

	var builderCalled bool
	builder := func(ctx context.Context, remote string, includePR bool) (engine.PushPlan, error) {
		require.Equal(t, "origin", remote)
		require.True(t, includePR)
		builderCalled = true
		return plan, nil
	}

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetIn(strings.NewReader("yes\n"))

	require.NoError(t, runPushFlow(cmd, eng, "origin", true, false, builder))
	require.True(t, builderCalled)
	require.Len(t, eng.executed, 1)
	require.Contains(t, out.String(), "Proceed? [yes/no]")
	require.Contains(t, out.String(), "Will push branch feature")
}

func TestRunPushFlowSkipsExecutionWhenNoWork(t *testing.T) {
	eng := &stubPushEngine{}
	plan := engine.PushPlan{
		Remote:  "origin",
		Actions: []engine.PushAction{{Branch: "main", IsBase: true}},
	}

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	require.NoError(t, runPushFlow(cmd, eng, "origin", true, false, func(context.Context, string, bool) (engine.PushPlan, error) {
		return plan, nil
	}))
	require.Empty(t, eng.executed)
}

func TestRunPushFlowSkipsConfirmationWhenConfigured(t *testing.T) {
	eng := &stubPushEngine{cfg: config.Config{SkipConfirm: true}}
	plan := engine.PushPlan{
		Remote: "origin",
		Actions: []engine.PushAction{
			{Branch: "main", IsBase: true},
			{Branch: "feature", Parent: "main", Push: true},
		},
	}

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(&bytes.Buffer{})

	require.NoError(t, runPushFlow(cmd, eng, "origin", true, false, func(context.Context, string, bool) (engine.PushPlan, error) {
		return plan, nil
	}))
	require.Len(t, eng.executed, 1)
	require.NotContains(t, out.String(), "Proceed? [yes/no]")
}

func TestRunPushFlowSkipsConfirmationWithForce(t *testing.T) {
	eng := &stubPushEngine{}
	plan := engine.PushPlan{
		Remote: "origin",
		Actions: []engine.PushAction{
			{Branch: "main", IsBase: true},
			{Branch: "feature", Parent: "main", Push: true},
		},
	}

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(&bytes.Buffer{})

	require.NoError(t, runPushFlow(cmd, eng, "origin", true, true, func(context.Context, string, bool) (engine.PushPlan, error) {
		return plan, nil
	}))
	require.Len(t, eng.executed, 1)
	require.NotContains(t, out.String(), "Proceed? [yes/no]")
}

func TestRunPushFlowPropagatesBuilderError(t *testing.T) {
	eng := &stubPushEngine{}
	expected := context.DeadlineExceeded

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := runPushFlow(cmd, eng, "origin", true, false, func(context.Context, string, bool) (engine.PushPlan, error) {
		return engine.PushPlan{}, expected
	})
	require.ErrorIs(t, err, expected)
	require.Empty(t, eng.executed)
}
