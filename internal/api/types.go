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

// VMInfo is the serialisable representation of a VM returned by the server.
type VMInfo struct {
	ID    string `json:"id"`
	State string `json:"state"`
	Image string `json:"image"`
}

// IDParams carries a single VM identifier.
type IDParams struct {
	ID string `json:"id"`
}
