package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUserOption(t *testing.T) {
	original := os.Args
	t.Cleanup(func() { os.Args = original })
	for _, command := range []string{"create", "start", "stop", "delete", "cleanup"} {
		os.Args = []string{"iqhost", command, "--id", "vm-001", "--user", "alice"}
		options, err := parseOptions()
		require.NoError(t, err)
		require.Equal(t, "alice", options.userID)
		os.Args = []string{"iqhost", command, "--id", "vm-001"}
		options, err = parseOptions()
		require.NoError(t, err)
		require.Equal(t, "unknown", options.userID)
	}
}
