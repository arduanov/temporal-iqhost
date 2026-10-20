package reconcile

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	enums "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	workflowpb "go.temporal.io/api/workflow/v1"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/mocks"

	"temporal-iqhost/internal/vm"
)

type repository struct{ fenced bool }

func (r *repository) Candidates(context.Context) ([]vm.Operation, error) {
	return []vm.Operation{{ID: "op", WorkflowID: "wf", RunID: "run"}}, nil
}
func (r *repository) Fence(context.Context, string) error { r.fenced = true; return nil }
func TestReconciliation(t *testing.T) {
	for _, state := range []enums.WorkflowExecutionStatus{enums.WORKFLOW_EXECUTION_STATUS_RUNNING, enums.WORKFLOW_EXECUTION_STATUS_COMPLETED, enums.WORKFLOW_EXECUTION_STATUS_FAILED, enums.WORKFLOW_EXECUTION_STATUS_TERMINATED, enums.WORKFLOW_EXECUTION_STATUS_TIMED_OUT} {
		t.Run(state.String(), func(t *testing.T) {
			r := &repository{}
			c := &mocks.Client{}
			c.On("DescribeWorkflowExecution", mock.Anything, "wf", "run").Return(&workflowservice.DescribeWorkflowExecutionResponse{WorkflowExecutionInfo: &workflowpb.WorkflowExecutionInfo{Status: state}}, nil)
			require.NoError(t, (&Reconciler{Store: r, Client: c}).Once(context.Background()))
			require.Equal(t, state != enums.WORKFLOW_EXECUTION_STATUS_RUNNING, r.fenced)
		})
	}
	for _, missing := range []bool{false, true} {
		r := &repository{}
		c := &mocks.Client{}
		e := serviceerror.NewUnavailable("offline")
		if missing {
			e = serviceerror.NewNotFound("expired")
		}
		c.On("DescribeWorkflowExecution", mock.Anything, "wf", "run").Return(nil, e)
		require.NoError(t, (&Reconciler{Store: r, Client: c}).Once(context.Background()))
		require.Equal(t, missing, r.fenced)
	}
}
