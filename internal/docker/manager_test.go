package docker

import (
	"bytes"
	"context"
	"encoding/binary"
	"testing"
	"time"

	"github.com/kariz/kariz/internal/models"
)

// buildMuxFrame constructs a Docker multiplexed stream frame with the given
// stream type (1=stdout, 2=stderr) and payload data.
func buildMuxFrame(streamType byte, data string) []byte {
	payload := []byte(data)
	header := make([]byte, 8)
	header[0] = streamType
	binary.BigEndian.PutUint32(header[4:8], uint32(len(payload)))
	return append(header, payload...)
}

func TestDemuxStream_StdoutOnly(t *testing.T) {
	var buf bytes.Buffer
	buf.Write(buildMuxFrame(1, "hello stdout"))

	ch := make(chan models.OutputChunk, 16)
	demuxStream(context.Background(), &buf, ch)

	chunks := collectChunks(ch)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0].Stream != "stdout" {
		t.Errorf("expected stream stdout, got %s", chunks[0].Stream)
	}
	if chunks[0].Data != "hello stdout" {
		t.Errorf("expected data 'hello stdout', got %q", chunks[0].Data)
	}
}

func TestDemuxStream_StderrOnly(t *testing.T) {
	var buf bytes.Buffer
	buf.Write(buildMuxFrame(2, "error output"))

	ch := make(chan models.OutputChunk, 16)
	demuxStream(context.Background(), &buf, ch)

	chunks := collectChunks(ch)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0].Stream != "stderr" {
		t.Errorf("expected stream stderr, got %s", chunks[0].Stream)
	}
	if chunks[0].Data != "error output" {
		t.Errorf("expected data 'error output', got %q", chunks[0].Data)
	}
}

func TestDemuxStream_MixedStreams(t *testing.T) {
	var buf bytes.Buffer
	buf.Write(buildMuxFrame(1, "line1"))
	buf.Write(buildMuxFrame(2, "err1"))
	buf.Write(buildMuxFrame(1, "line2"))

	ch := make(chan models.OutputChunk, 16)
	demuxStream(context.Background(), &buf, ch)

	chunks := collectChunks(ch)
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}

	expected := []struct {
		stream string
		data   string
	}{
		{"stdout", "line1"},
		{"stderr", "err1"},
		{"stdout", "line2"},
	}

	for i, exp := range expected {
		if chunks[i].Stream != exp.stream {
			t.Errorf("chunk %d: expected stream %s, got %s", i, exp.stream, chunks[i].Stream)
		}
		if chunks[i].Data != exp.data {
			t.Errorf("chunk %d: expected data %q, got %q", i, exp.data, chunks[i].Data)
		}
	}
}

func TestDemuxStream_EmptyPayload(t *testing.T) {
	var buf bytes.Buffer
	// Frame with zero-length payload should be skipped
	buf.Write(buildMuxFrame(1, ""))
	buf.Write(buildMuxFrame(1, "after empty"))

	ch := make(chan models.OutputChunk, 16)
	demuxStream(context.Background(), &buf, ch)

	chunks := collectChunks(ch)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk (empty skipped), got %d", len(chunks))
	}
	if chunks[0].Data != "after empty" {
		t.Errorf("expected 'after empty', got %q", chunks[0].Data)
	}
}

func TestDemuxStream_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// Create a reader that blocks forever
	pr, _ := newBlockingPipe()

	ch := make(chan models.OutputChunk, 16)
	done := make(chan struct{})
	go func() {
		demuxStream(ctx, pr, ch)
		close(done)
	}()

	// Cancel the context
	cancel()

	// demuxStream should exit promptly
	select {
	case <-done:
		// Good — exited after cancellation
	case <-time.After(2 * time.Second):
		t.Fatal("demuxStream did not exit after context cancellation")
	}
}

func TestDemuxStream_UnknownStreamType(t *testing.T) {
	var buf bytes.Buffer
	// Stream type 0 (unknown) should default to stdout
	buf.Write(buildMuxFrame(0, "unknown type"))

	ch := make(chan models.OutputChunk, 16)
	demuxStream(context.Background(), &buf, ch)

	chunks := collectChunks(ch)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0].Stream != "stdout" {
		t.Errorf("expected stream stdout for unknown type, got %s", chunks[0].Stream)
	}
}

func TestDemuxStream_LargePayload(t *testing.T) {
	// Test with a payload larger than typical buffer sizes
	largeData := make([]byte, 65536)
	for i := range largeData {
		largeData[i] = byte('A' + (i % 26))
	}

	var buf bytes.Buffer
	buf.Write(buildMuxFrame(1, string(largeData)))

	ch := make(chan models.OutputChunk, 16)
	demuxStream(context.Background(), &buf, ch)

	chunks := collectChunks(ch)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if len(chunks[0].Data) != 65536 {
		t.Errorf("expected 65536 bytes, got %d", len(chunks[0].Data))
	}
}

func TestDemuxStream_TimestampSet(t *testing.T) {
	var buf bytes.Buffer
	buf.Write(buildMuxFrame(1, "timestamped"))

	before := time.Now()
	ch := make(chan models.OutputChunk, 16)
	demuxStream(context.Background(), &buf, ch)
	after := time.Now()

	chunks := collectChunks(ch)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0].Timestamp.Before(before) || chunks[0].Timestamp.After(after) {
		t.Errorf("timestamp %v not between %v and %v", chunks[0].Timestamp, before, after)
	}
}

func TestDemuxStream_PartialHeader(t *testing.T) {
	// Only 4 bytes of header — should exit gracefully
	var buf bytes.Buffer
	buf.Write([]byte{1, 0, 0, 0})

	ch := make(chan models.OutputChunk, 16)
	demuxStream(context.Background(), &buf, ch)

	chunks := collectChunks(ch)
	if len(chunks) != 0 {
		t.Fatalf("expected 0 chunks for partial header, got %d", len(chunks))
	}
}

func TestDemuxStream_EmptyReader(t *testing.T) {
	var buf bytes.Buffer

	ch := make(chan models.OutputChunk, 16)
	demuxStream(context.Background(), &buf, ch)

	chunks := collectChunks(ch)
	if len(chunks) != 0 {
		t.Fatalf("expected 0 chunks for empty reader, got %d", len(chunks))
	}
}

// collectChunks drains a closed channel into a slice.
func collectChunks(ch <-chan models.OutputChunk) []models.OutputChunk {
	var chunks []models.OutputChunk
	for chunk := range ch {
		chunks = append(chunks, chunk)
	}
	return chunks
}

// newBlockingPipe returns a reader that blocks on Read until the context is done.
// It uses a pipe where we never write to the writer.
func newBlockingPipe() (*blockingReader, func()) {
	ctx, cancel := context.WithCancel(context.Background())
	return &blockingReader{ctx: ctx}, cancel
}

type blockingReader struct {
	ctx context.Context
}

func (r *blockingReader) Read(p []byte) (int, error) {
	<-r.ctx.Done()
	return 0, r.ctx.Err()
}
