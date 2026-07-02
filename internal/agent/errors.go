package agent

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

type ErrorKind string

const (
	KindUnauthorized      ErrorKind = "unauthorized"
	KindDaemonUnreachable ErrorKind = "daemon_unreachable"
	KindNotFound          ErrorKind = "not_found"
	KindInvalid           ErrorKind = "invalid"
	KindInternal          ErrorKind = "internal"
	KindNotSupported      ErrorKind = "not_supported"
)

type ErrorBody struct {
	Error ErrorDetail `json:"error"`
}

type ErrorDetail struct {
	Message string    `json:"message"`
	Kind    ErrorKind `json:"kind"`
}

func WriteError(w http.ResponseWriter, status int, kind ErrorKind, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorBody{
		Error: ErrorDetail{
			Message: msg,
			Kind:    kind,
		},
	})
}

func MapError(err error) (int, ErrorKind) {
	if err == nil {
		return http.StatusOK, ""
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "not found"), strings.Contains(msg, "no such"):
		return http.StatusNotFound, KindNotFound
	case strings.Contains(msg, "invalid"), strings.Contains(msg, "bad request"):
		return http.StatusBadRequest, KindInvalid
	case strings.Contains(msg, "authentication"), strings.Contains(msg, "unauthorized"):
		return http.StatusBadGateway, KindDaemonUnreachable
	default:
		return http.StatusInternalServerError, KindInternal
	}
}

var errDaemonUnreachable = errors.New("daemon unreachable")

func WriteMappedError(w http.ResponseWriter, err error) {
	if errors.Is(err, errNotSupported) {
		WriteError(w, http.StatusBadRequest, KindNotSupported, err.Error())
		return
	}
	if errors.Is(err, errDaemonUnreachable) {
		WriteError(w, http.StatusBadGateway, KindDaemonUnreachable, err.Error())
		return
	}
	status, kind := MapError(err)
	WriteError(w, status, kind, err.Error())
}
