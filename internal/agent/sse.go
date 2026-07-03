package agent

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type SSEEvent struct {
	Name string
	Data []byte
}

func WriteSSEEvent(w io.Writer, name string, data []byte) error {
	if name != "" {
		if _, err := fmt.Fprintf(w, "event: %s\n", name); err != nil {
			return fmt.Errorf("write sse event name: %w", err)
		}
	}
	for _, line := range strings.Split(string(data), "\n") {
		if _, err := fmt.Fprintf(w, "data: %s\n", line); err != nil {
			return fmt.Errorf("write sse data: %w", err)
		}
	}
	_, err := io.WriteString(w, "\n")
	if err != nil {
		return fmt.Errorf("finish sse event: %w", err)
	}
	return nil
}

func WriteSSEJSON(w io.Writer, name string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal sse json: %w", err)
	}
	return WriteSSEEvent(w, name, data)
}

func EncodeChunk(chunk []byte) []byte {
	out := make([]byte, base64.StdEncoding.EncodedLen(len(chunk)))
	base64.StdEncoding.Encode(out, chunk)
	return out
}

type sseChunkWriter struct {
	w       io.Writer
	flusher interface{ Flush() }
}

func (w *sseChunkWriter) Write(p []byte) (int, error) {
	if err := WriteSSEEvent(w.w, "chunk", EncodeChunk(p)); err != nil {
		return 0, err
	}
	if w.flusher != nil {
		w.flusher.Flush()
	}
	return len(p), nil
}
