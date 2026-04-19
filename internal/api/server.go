package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"

	"github.com/AitorConS/unikernel-engine/internal/vm"
)

// Server listens on a Unix socket and dispatches JSON-RPC requests to a
// vm.Manager.
type Server struct {
	mgr      vm.Manager
	listener net.Listener
}

// NewServer creates a Server that will listen on socketPath.
// Any existing socket file at socketPath is removed before binding.
func NewServer(mgr vm.Manager, socketPath string) (*Server, error) {
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("api server remove stale socket: %w", err)
	}
	l, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("api server listen %s: %w", socketPath, err)
	}
	return &Server{mgr: mgr, listener: l}, nil
}

// Serve accepts connections and handles them until ctx is cancelled.
func (s *Server) Serve(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		if err := s.listener.Close(); err != nil {
			slog.Warn("api server close listener", "err", err)
		}
	}()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("api server accept: %w", err)
		}
		go s.handle(ctx, conn)
	}
}

func (s *Server) handle(ctx context.Context, conn net.Conn) {
	defer func() {
		if err := conn.Close(); err != nil {
			slog.Warn("api server close conn", "err", err)
		}
	}()
	dec := json.NewDecoder(conn)
	enc := json.NewEncoder(conn)
	for dec.More() {
		var req Request
		if err := dec.Decode(&req); err != nil {
			return
		}
		result, rpcErr := s.dispatch(ctx, &req)
		resp := Response{JSONRPC: "2.0", ID: req.ID}
		if rpcErr != nil {
			resp.Error = rpcErr
		} else {
			raw, err := json.Marshal(result)
			if err != nil {
				slog.Warn("api server marshal result", "err", err)
				return
			}
			resp.Result = raw
		}
		if err := enc.Encode(resp); err != nil {
			slog.Warn("api server encode response", "err", err)
			return
		}
	}
}

func (s *Server) dispatch(ctx context.Context, req *Request) (any, *RPCError) {
	switch req.Method {
	case "VM.Run":
		return s.handleRun(ctx, req.Params)
	case "VM.Stop":
		return s.handleStop(ctx, req.Params)
	case "VM.Remove":
		return s.handleRemove(ctx, req.Params)
	case "VM.List":
		return s.handleList(ctx)
	case "VM.Get":
		return s.handleGet(req.Params)
	default:
		return nil, &RPCError{Code: -32601, Message: "method not found: " + req.Method}
	}
}

func (s *Server) handleRun(ctx context.Context, params json.RawMessage) (any, *RPCError) {
	var p RunParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &RPCError{Code: -32602, Message: "invalid params: " + err.Error()}
	}
	cfg := vm.Config{
		ImagePath:   p.ImagePath,
		Memory:      p.Memory,
		CPUs:        p.CPUs,
		NetworkName: p.NetworkName,
	}
	v, err := s.mgr.Create(ctx, cfg)
	if err != nil {
		return nil, &RPCError{Code: -32000, Message: err.Error()}
	}
	if err := s.mgr.Start(ctx, v.ID); err != nil {
		return nil, &RPCError{Code: -32000, Message: err.Error()}
	}
	return toInfo(v), nil
}

func (s *Server) handleStop(ctx context.Context, params json.RawMessage) (any, *RPCError) {
	var p IDParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &RPCError{Code: -32602, Message: "invalid params: " + err.Error()}
	}
	if err := s.mgr.Stop(ctx, p.ID); err != nil {
		return nil, &RPCError{Code: -32000, Message: err.Error()}
	}
	return map[string]string{"status": "ok"}, nil
}

func (s *Server) handleRemove(ctx context.Context, params json.RawMessage) (any, *RPCError) {
	var p IDParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &RPCError{Code: -32602, Message: "invalid params: " + err.Error()}
	}
	if err := s.mgr.Remove(ctx, p.ID); err != nil {
		return nil, &RPCError{Code: -32000, Message: err.Error()}
	}
	return map[string]string{"status": "ok"}, nil
}

func (s *Server) handleList(_ context.Context) (any, *RPCError) {
	vms := s.mgr.List()
	infos := make([]VMInfo, len(vms))
	for i, v := range vms {
		infos[i] = toInfo(v)
	}
	return infos, nil
}

func (s *Server) handleGet(params json.RawMessage) (any, *RPCError) {
	var p IDParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &RPCError{Code: -32602, Message: "invalid params: " + err.Error()}
	}
	v, err := s.mgr.Get(p.ID)
	if err != nil {
		return nil, &RPCError{Code: -32000, Message: err.Error()}
	}
	return toInfo(v), nil
}

func toInfo(v *vm.VM) VMInfo {
	return VMInfo{
		ID:    v.ID,
		State: string(v.GetState()),
		Image: v.Cfg.ImagePath,
	}
}
