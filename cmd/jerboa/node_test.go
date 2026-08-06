//go:build linux

package main

import (
	"encoding/json"
	"testing"

	"github.com/AitorConS/jerboa/internal/api"
	"github.com/stretchr/testify/require"
)

func TestNodeListCmd_Disabled(t *testing.T) {
	client, socketPath := startDaemon(t)
	storePath := t.TempDir()

	_ = execRootExpectError(t, socketPath, storePath, "node", "ls")
	_ = client
}

// TestNodeListResponse_JSONRoundTrip exercises the actual wire contract of the
// node-list RPC response instead of merely reading back struct fields the test
// just set. It pins the JSON field names (a rename would silently break every
// client) and verifies marshal→unmarshal is lossless, including the 64-bit
// memory capacity where a wrong tag or int32 truncation would show up.
func TestNodeListResponse_JSONRoundTrip(t *testing.T) {
	orig := api.NodeListResponse{
		Nodes: []api.NodeRow{
			{ID: "node-1", Addr: "10.0.0.1:7946", Status: "alive", VMCount: 3, CPUCap: 8, MemCap: 17179869184, LastSeen: "2026-05-16T10:00:00Z"},
		},
	}

	data, err := json.Marshal(orig)
	require.NoError(t, err)

	// The wire field names are part of the contract with remote clients.
	var wire map[string]any
	require.NoError(t, json.Unmarshal(data, &wire))
	nodes, ok := wire["nodes"].([]any)
	require.True(t, ok, "response must serialize a \"nodes\" array")
	require.Len(t, nodes, 1)
	row := nodes[0].(map[string]any)
	for _, field := range []string{"id", "addr", "status", "vm_count", "cpu_capacity", "mem_capacity_bytes", "last_seen"} {
		require.Contains(t, row, field, "wire contract must keep field %q", field)
	}

	// Round-trip must reproduce the value exactly, 64-bit fields included.
	var got api.NodeListResponse
	require.NoError(t, json.Unmarshal(data, &got))
	require.Equal(t, orig, got)
	require.Equal(t, int64(17179869184), got.Nodes[0].MemCap)
}
