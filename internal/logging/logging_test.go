package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestJSONAndLevel(t *testing.T) {
	var b bytes.Buffer
	l := New(&b, "INFO").With("component", "cli")
	l.Debug("hidden")
	l.Info("accepted", "event", "command_accepted", "operation_id", "op")
	var entry map[string]interface{}
	require.NoError(t, json.Unmarshal(b.Bytes(), &entry))
	require.Equal(t, "cli", entry["component"])
	require.Equal(t, "op", entry["operation_id"])
	require.Equal(t, "INFO", entry["level"])
}

func TestContextLogger(t *testing.T) {
	var output bytes.Buffer
	logger := New(&output, "INFO").With("workflow_id", "workflow", "run_id", "run", "attempt", 2)
	ctx := WithLogger(context.Background(), logger)
	FromContext(ctx).Info("step", "operation_id", "operation")
	require.Contains(t, output.String(), `"workflow_id":"workflow"`)
	require.Contains(t, output.String(), `"run_id":"run"`)
	require.Contains(t, output.String(), `"attempt":2`)
	require.Same(t, slog.Default(), FromContext(context.Background()))
}
