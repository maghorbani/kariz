package stream

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kariz/kariz/internal/models"
)

// SSEHandler handles SSE streaming endpoints.
type SSEHandler struct {
	streamMgr StreamManager
}

// NewSSEHandler creates a new SSEHandler with the given StreamManager.
func NewSSEHandler(streamMgr StreamManager) *SSEHandler {
	return &SSEHandler{
		streamMgr: streamMgr,
	}
}

// RegisterRoutes registers SSE streaming routes on the given router group.
func (h *SSEHandler) RegisterRoutes(rg *gin.RouterGroup, authMiddleware gin.HandlerFunc) {
	executions := rg.Group("/executions")
	executions.Use(authMiddleware)
	executions.GET("/:id/stream", h.StreamExecution)
}

// StreamExecution handles GET /api/executions/:id/stream.
// It sets SSE headers, subscribes to the execution stream, and writes events
// until the execution completes or the client disconnects.
func (h *SSEHandler) StreamExecution(c *gin.Context) {
	executionID := c.Param("id")
	lastEventID := c.GetHeader("Last-Event-ID")

	// Set SSE headers.
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	// Subscribe to the stream.
	eventCh, unsubscribe := h.streamMgr.Subscribe(executionID, lastEventID)
	defer unsubscribe()

	// Get the client disconnect context.
	clientGone := c.Request.Context().Done()

	c.Status(http.StatusOK)

	keepalive := time.NewTicker(30 * time.Second)
	defer keepalive.Stop()

	for {
		select {
		case <-clientGone:
			return
		case <-keepalive.C:
			_, _ = fmt.Fprintf(c.Writer, ": keepalive\n\n")
			c.Writer.Flush()
		case event, ok := <-eventCh:
			if !ok {
				return
			}
			h.writeSSEEvent(c, event)
		}
	}
}

// writeSSEEvent writes a single SSE event to the response writer.
func (h *SSEHandler) writeSSEEvent(c *gin.Context, event models.SSEEvent) {
	_, _ = fmt.Fprintf(c.Writer, "id: %s\nevent: %s\ndata: %s\n\n", event.ID, event.Event, event.Data)
	c.Writer.Flush()
}
