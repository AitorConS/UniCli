package agent

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSSEChunkEncoding(t *testing.T) {
	var buf bytes.Buffer
	w := &sseChunkWriter{w: &buf}

	n, err := w.Write([]byte{0x00, 0xff, '\n'})

	require.NoError(t, err)
	require.Equal(t, 3, n)
	require.Equal(t, "event: chunk\ndata: AP8K\n\n", buf.String())
}
