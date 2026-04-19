package registry

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"

	"github.com/AitorConS/unikernel-engine/internal/image"
)

// Server is an HTTP image registry backed by an image.Store.
//
// Routes:
//
//	GET    /v2/images           — list all images
//	GET    /v2/images/{ref}     — get manifest by name:tag or sha256
//	GET    /v2/images/{ref}/disk — download raw disk image
//	POST   /v2/images           — push image (multipart: manifest + disk)
//	DELETE /v2/images/{ref}     — remove image
type Server struct {
	store *image.Store
}

// NewServer returns a Server backed by store.
func NewServer(store *image.Store) *Server {
	return &Server{store: store}
}

// Handler returns an http.Handler for the registry API.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v2/images", s.handleList)
	mux.HandleFunc("POST /v2/images", s.handlePush)
	mux.HandleFunc("GET /v2/images/{ref}", s.handleGetManifest)
	mux.HandleFunc("GET /v2/images/{ref}/disk", s.handleGetDisk)
	mux.HandleFunc("DELETE /v2/images/{ref}", s.handleRemove)
	return mux
}

func (s *Server) handleList(w http.ResponseWriter, _ *http.Request) {
	list, err := s.store.List()
	if err != nil {
		httpErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleGetManifest(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("ref")
	m, _, err := s.store.Get(ref)
	if err != nil {
		httpErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (s *Server) handleGetDisk(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("ref")
	_, diskPath, err := s.store.Get(ref)
	if err != nil {
		httpErr(w, http.StatusNotFound, err.Error())
		return
	}
	f, err := os.Open(diskPath)
	if err != nil {
		httpErr(w, http.StatusInternalServerError, "open disk: "+err.Error())
		return
	}
	defer func() {
		if err := f.Close(); err != nil {
			slog.Warn("registry: close disk file", "err", err)
		}
	}()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, f); err != nil {
		slog.Warn("registry: stream disk", "err", err)
	}
}

func (s *Server) handlePush(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(512 << 20); err != nil {
		httpErr(w, http.StatusBadRequest, "parse multipart: "+err.Error())
		return
	}
	manifestJSON := r.FormValue("manifest")
	if manifestJSON == "" {
		httpErr(w, http.StatusBadRequest, "manifest field is required")
		return
	}
	m, err := image.Parse([]byte(manifestJSON))
	if err != nil {
		httpErr(w, http.StatusBadRequest, "invalid manifest: "+err.Error())
		return
	}
	file, _, err := r.FormFile("disk")
	if err != nil {
		httpErr(w, http.StatusBadRequest, "disk field is required: "+err.Error())
		return
	}
	defer func() {
		if err := file.Close(); err != nil {
			slog.Warn("registry: close upload", "err", err)
		}
	}()

	tmp, err := writeTempFile(file)
	if err != nil {
		httpErr(w, http.StatusInternalServerError, "store upload: "+err.Error())
		return
	}
	defer func() { _ = os.Remove(tmp) }()

	if err := s.store.Put(m.Name, m.Tag, m, tmp); err != nil {
		httpErr(w, http.StatusInternalServerError, "store put: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

func (s *Server) handleRemove(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("ref")
	if err := s.store.Remove(ref); err != nil {
		httpErr(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func httpErr(w http.ResponseWriter, code int, msg string) {
	http.Error(w, fmt.Sprintf(`{"error":%q}`, msg), code)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("registry: encode response", "err", err)
	}
}

func writeTempFile(r io.Reader) (string, error) {
	f, err := os.CreateTemp("", "uni-registry-upload-*.img")
	if err != nil {
		return "", fmt.Errorf("create temp: %w", err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			_ = err // best effort
		}
	}()
	if _, err := io.Copy(f, r); err != nil {
		_ = os.Remove(f.Name())
		return "", fmt.Errorf("write temp: %w", err)
	}
	return f.Name(), nil
}
