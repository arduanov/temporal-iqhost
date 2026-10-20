package orchestration

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	enums "go.temporal.io/api/enums/v1"
	historypb "go.temporal.io/api/history/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"

	"temporal-iqhost/internal/logging"
)

// Set TEST_WORKFLOW_ID to a completed Lifecycle execution from the Compose smoke.
func TestLifecycleReplay(t *testing.T) {
	if historyFile := os.Getenv("TEST_WORKFLOW_HISTORY"); historyFile != "" {
		var logs bytes.Buffer
		logger := logging.Temporal{Log: logging.New(&logs, "INFO")}
		replayer := worker.NewWorkflowReplayer()
		replayer.RegisterWorkflow(Lifecycle)
		require.NoError(t, replayer.ReplayWorkflowHistoryFromJSONFile(logger, historyFile))
		require.NotContains(t, logs.String(), "operation_started")
		return
	}
	workflowID := os.Getenv("TEST_WORKFLOW_ID")
	if workflowID == "" {
		t.Skip("TEST_WORKFLOW_ID is required for replay against Temporal")
	}
	address := os.Getenv("TEST_TEMPORAL_ADDRESS")
	if address == "" {
		address = "localhost:7233"
	}
	var logs bytes.Buffer
	logger := logging.Temporal{Log: logging.New(&logs, "INFO")}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c, e := client.DialContext(ctx, client.Options{HostPort: address, Logger: logger})
	require.NoError(t, e)
	defer c.Close()
	iterator := c.GetWorkflowHistory(ctx, workflowID, "", false, enums.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT)
	history := &historypb.History{}
	for iterator.HasNext() {
		event, e := iterator.Next()
		require.NoError(t, e)
		history.Events = append(history.Events, event)
	}
	replayer := worker.NewWorkflowReplayer()
	replayer.RegisterWorkflow(Lifecycle)
	require.NoError(t, replayer.ReplayWorkflowHistory(logger, history))
	require.NotContains(t, logs.String(), "operation_started", "workflow logs must not repeat during replay")
}
