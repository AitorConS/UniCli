package agent

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMapError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantKind   ErrorKind
	}{
		{name: "not found", err: errors.New("vm not found"), wantStatus: http.StatusNotFound, wantKind: KindNotFound},
		{name: "invalid", err: errors.New("invalid params"), wantStatus: http.StatusBadRequest, wantKind: KindInvalid},
		{name: "daemon auth", err: errors.New("authentication failed"), wantStatus: http.StatusBadGateway, wantKind: KindDaemonUnreachable},
		{name: "internal", err: errors.New("boom"), wantStatus: http.StatusInternalServerError, wantKind: KindInternal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStatus, gotKind := MapError(tt.err)
			require.Equal(t, tt.wantStatus, gotStatus)
			require.Equal(t, tt.wantKind, gotKind)
		})
	}
}

func TestWriteMappedError_NilIsNoOp(t *testing.T) {
	rr := httptestResponse()

	// Regression: a nil error used to reach err.Error() and panic.
	WriteMappedError(rr, nil)

	require.Equal(t, http.StatusOK, rr.Code)
	require.Empty(t, rr.Body.String())
}

func TestWriteMappedError_DaemonUnreachable(t *testing.T) {
	rr := httptestResponse()

	WriteMappedError(rr, fmt.Errorf("%w: dial tcp: refused", errDaemonUnreachable))

	require.Equal(t, http.StatusBadGateway, rr.Code)
	require.Contains(t, rr.Body.String(), `"kind":"daemon_unreachable"`)
}

func TestWriteMappedError_NotSupported(t *testing.T) {
	rr := httptestResponse()

	WriteMappedError(rr, fmt.Errorf("daemon start: %w", errNotSupported))

	require.Equal(t, http.StatusBadRequest, rr.Code)
	require.Contains(t, rr.Body.String(), `"kind":"not_supported"`)
}
