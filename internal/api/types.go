package api

import "encoding/json"

// Request is a JSON-RPC 2.0 request envelope.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response is a JSON-RPC 2.0 response envelope.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCError carries a JSON-RPC error code and message.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// RunParams are the parameters for the VM.Run method.
type RunParams struct {
	ImagePath   string `json:"image_path"`
	Memory      string `json:"memory"`
	CPUs        int    `json:"cpus"`
	NetworkName string `json:"network_name,omitempty"`
}

// StopParams are the parameters for VM.Stop.
type StopParams struct {
	// ID is the VM identifier.
	ID string `json:"id"`
	// Force skips graceful shutdown and sends SIGKILL immediately.
	Force bool `json:"force,omitempty"`
}

// SignalParams are the parameters for VM.Signal.
type SignalParams struct {
	// ID is the VM identifier.
	ID string `json:"id"`
	// Signal is the signal name (e.g. "SIGTERM") or number string (e.g. "15").
	Signal string `json:"signal"`
}

// VMInfo is the compact serialisable representation of a VM.
type VMInfo struct {
	ID    string `json:"id"`
	State string `json:"state"`
	Image string `json:"image"`
}

// VMDetail is the full serialisable representation of a VM.
type VMDetail struct {
	ID        string  `json:"id"`
	State     string  `json:"state"`
	Image     string  `json:"image"`
	Memory    string  `json:"memory"`
	CPUs      int     `json:"cpus"`
	CreatedAt string  `json:"created_at"`
	StartedAt *string `json:"started_at,omitempty"`
	StoppedAt *string `json:"stopped_at,omitempty"`
}

// LogsResponse carries the captured serial console output for a VM.
type LogsResponse struct {
	ID   string `json:"id"`
	Logs string `json:"logs"`
}

// IDParams carries a single VM identifier.
type IDParams struct {
	ID string `json:"id"`
}
