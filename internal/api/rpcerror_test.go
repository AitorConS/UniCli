package api

import "testing"

// TestRPCError_Error is the F-004 regression: application/server errors must
// surface only their message (no "rpc error -32000:" plumbing), while
// protocol-level codes keep the code so a client/daemon mismatch is diagnosable.
func TestRPCError_Error(t *testing.T) {
	cases := []struct {
		name string
		err  *RPCError
		want string
	}{
		{"application error hides code", &RPCError{Code: -32000, Message: `vm "web" not found`}, `vm "web" not found`},
		{"top of application range hides code", &RPCError{Code: -32099, Message: "boom"}, "boom"},
		{"method not found keeps code", &RPCError{Code: -32601, Message: "method not found: Foo.Bar"}, "method not found: Foo.Bar (rpc -32601)"},
		{"invalid params keeps code", &RPCError{Code: -32602, Message: "bad params"}, "bad params (rpc -32602)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.err.Error(); got != tc.want {
				t.Fatalf("Error() = %q, want %q", got, tc.want)
			}
		})
	}
	if (*RPCError)(nil).Error() != "" {
		t.Fatal("nil RPCError should render as empty string")
	}
}
