package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
)

// Client connects to a unid server over a Unix socket.
type Client struct {
	conn net.Conn
	mu   sync.Mutex
	enc  *json.Encoder
	dec  *json.Decoder
	seq  atomic.Int64
}

// Dial connects to the unid server at socketPath.
func Dial(socketPath string) (*Client, error) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("api client dial %s: %w", socketPath, err)
	}
	return &Client{
		conn: conn,
		enc:  json.NewEncoder(conn),
		dec:  json.NewDecoder(conn),
	}, nil
}

// Close closes the underlying connection.
func (c *Client) Close() error {
	if err := c.conn.Close(); err != nil {
		return fmt.Errorf("api client close: %w", err)
	}
	return nil
}

// Run creates and starts a VM, returning its info.
func (c *Client) Run(_ context.Context, p RunParams) (VMInfo, error) {
	var info VMInfo
	if err := c.call("VM.Run", p, &info); err != nil {
		return VMInfo{}, fmt.Errorf("client run: %w", err)
	}
	return info, nil
}

// Stop stops the VM with the given id.
func (c *Client) Stop(_ context.Context, id string) error {
	if err := c.call("VM.Stop", IDParams{ID: id}, nil); err != nil {
		return fmt.Errorf("client stop: %w", err)
	}
	return nil
}

// Remove removes the VM with the given id.
func (c *Client) Remove(_ context.Context, id string) error {
	if err := c.call("VM.Remove", IDParams{ID: id}, nil); err != nil {
		return fmt.Errorf("client remove: %w", err)
	}
	return nil
}

// List returns all VMs known to the daemon.
func (c *Client) List(_ context.Context) ([]VMInfo, error) {
	var infos []VMInfo
	if err := c.call("VM.List", nil, &infos); err != nil {
		return nil, fmt.Errorf("client list: %w", err)
	}
	return infos, nil
}

// Get returns the VM with the given id.
func (c *Client) Get(_ context.Context, id string) (VMInfo, error) {
	var info VMInfo
	if err := c.call("VM.Get", IDParams{ID: id}, &info); err != nil {
		return VMInfo{}, fmt.Errorf("client get: %w", err)
	}
	return info, nil
}

func (c *Client) call(method string, params any, out any) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	id := c.seq.Add(1)
	raw, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("marshal params: %w", err)
	}
	req := Request{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  json.RawMessage(raw),
	}
	if err := c.enc.Encode(req); err != nil {
		return fmt.Errorf("encode request: %w", err)
	}
	var resp Response
	if err := c.dec.Decode(&resp); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	if resp.Error != nil {
		return fmt.Errorf("rpc error %d: %s", resp.Error.Code, resp.Error.Message)
	}
	if out != nil && resp.Result != nil {
		if err := json.Unmarshal(resp.Result, out); err != nil {
			return fmt.Errorf("unmarshal result: %w", err)
		}
	}
	return nil
}
