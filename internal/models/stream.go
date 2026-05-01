package models

// SSEEvent represents a Server-Sent Event for real-time output streaming.
type SSEEvent struct {
	ID    string `json:"id"`
	Event string `json:"event"`
	Data  string `json:"data"`
}
