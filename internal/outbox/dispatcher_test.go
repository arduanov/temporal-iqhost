package outbox

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	common "go.temporal.io/api/common/v1"
	enums "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	workflowpb "go.temporal.io/api/workflow/v1"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/converter"
	"go.temporal.io/sdk/mocks"

	"temporal-iqhost/internal/visibility"
	"temporal-iqhost/internal/vm"
)

type repository struct {
	delivery  *vm.Delivery
	delivered string
	retry     bool
}

func (r *repository) Claim(context.Context) (*vm.Delivery, error) { return r.delivery, nil }
func (r *repository) Operation(context.Context, string) (vm.Operation, error) {
	return r.delivery.Operation, nil
}

func (r *repository) Delivered(_ context.Context, _ vm.Delivery, run string) error {
	r.delivered = run
	return nil
}

func (r *repository) RetryDelivery(context.Context, vm.Delivery, time.Time) error {
	r.retry = true
	return nil
}

func TestDeliveryDuplicateAndUnavailable(t *testing.T) {
	for _, mode := range []string{"success", "already-started", "mismatched", "unavailable", "terminal"} {
		t.Run(mode, func(t *testing.T) {
			r := &repository{delivery: &vm.Delivery{Operation: vm.Operation{ID: "op", WorkflowID: "vm-operation/op", Status: vm.Pending}}}
			c := &mocks.Client{}
			switch mode {
			case "terminal":
				r.delivery.Operation.Status = vm.Succeeded
			case "success":
				run := &mocks.WorkflowRun{}
				run.On("GetRunID").Return("run")
				c.On("ExecuteWorkflow", mock.Anything, mock.Anything, mock.Anything, "op").Return(run, nil)
			case "unavailable":
				c.On("ExecuteWorkflow", mock.Anything, mock.Anything, mock.Anything, "op").Return(nil, serviceerror.NewUnavailable("offline"))
			default:
				c.On("ExecuteWorkflow", mock.Anything, mock.Anything, mock.Anything, "op").Return(nil, &serviceerror.WorkflowExecutionAlreadyStarted{RunId: "run"})
				oid := "op"
				if mode == "mismatched" {
					oid = "other"
				}
				payload, e := converter.GetDefaultDataConverter().ToPayload(oid)
				require.NoError(t, e)
				c.On("DescribeWorkflowExecution", mock.Anything, "vm-operation/op", "run").Return(&workflowservice.DescribeWorkflowExecutionResponse{WorkflowExecutionInfo: &workflowpb.WorkflowExecutionInfo{Execution: &common.WorkflowExecution{WorkflowId: "vm-operation/op", RunId: "run"}, Type: &common.WorkflowType{Name: "Lifecycle"}, Status: enums.WORKFLOW_EXECUTION_STATUS_RUNNING, Memo: &common.Memo{Fields: map[string]*common.Payload{"operation_id": payload}}}}, nil)
			}
			require.NoError(t, (&Dispatcher{Store: r, Client: c}).Once(context.Background()))
			if mode == "success" || mode == "already-started" {
				require.Equal(t, "run", r.delivered)
			} else {
				require.Empty(t, r.delivered)
			}
			require.Equal(t, mode == "mismatched" || mode == "unavailable", r.retry)
			c.AssertExpectations(t)
		})
	}
}

func TestDeliveryMetadata(t *testing.T) {
	operation := vm.Operation{
		ID: "op", VMID: "vm-001", UserID: "alice", Kind: vm.StopVM,
		WorkflowID: "vm-operation/stop/vm-001/op", Status: vm.Pending,
	}
	store := &repository{delivery: &vm.Delivery{Operation: operation}}
	temporalClient := &mocks.Client{}
	run := &mocks.WorkflowRun{}
	run.On("GetRunID").Return("run")
	temporalClient.On("ExecuteWorkflow", mock.Anything, mock.MatchedBy(func(options client.StartWorkflowOptions) bool {
		require.Equal(t, operation.WorkflowID, options.ID)
		require.Equal(t, map[string]interface{}{
			"operation_id": "op", "operation_kind": "stop", "vm_id": "vm-001", "user_id": "alice",
		}, options.Memo)
		for key, expected := range map[string]string{"OperationKind": "stop", "VMId": "vm-001", "UserId": "alice"} {
			attributeKey := visibility.OperationKind
			if key == "VMId" {
				attributeKey = visibility.VMID
			}
			if key == "UserId" {
				attributeKey = visibility.UserID
			}
			actual, exists := options.TypedSearchAttributes.GetKeyword(attributeKey)
			require.True(t, exists)
			require.Equal(t, expected, actual)
		}
		return true
	}), mock.Anything, "op").Return(run, nil)
	require.NoError(t, (&Dispatcher{Store: store, Client: temporalClient}).Once(context.Background()))
	require.Equal(t, "run", store.delivered)
	temporalClient.AssertExpectations(t)
}
