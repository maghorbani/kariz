package models

import (
	"encoding/json"
	"time"
)

// ExecutionStatus represents the current state of a command execution.
type ExecutionStatus string

const (
	StatusQueued    ExecutionStatus = "queued"
	StatusRunning   ExecutionStatus = "running"
	StatusCompleted ExecutionStatus = "completed"
	StatusFailed    ExecutionStatus = "failed"
	StatusTimedOut  ExecutionStatus = "timed_out"
	StatusCancelled ExecutionStatus = "cancelled"
)

// ExecutionRecord captures the details and outcome of a single command execution.
type ExecutionRecord struct {
	ID          string              `json:"id" db:"id"`
	CommandID   string              `json:"command_id" db:"command_id"`
	CommandName string              `json:"command_name" db:"command_name"`
	UserID      string              `json:"user_id" db:"user_id"`
	Parameters  json.RawMessage     `json:"parameters" db:"parameters"`
	Status      ExecutionStatus     `json:"status" db:"status"`
	ExitCode    *int                `json:"exit_code" db:"exit_code"`
	Stdout      string              `json:"stdout" db:"stdout"`
	Stderr      string              `json:"stderr" db:"stderr"`
	ContainerID *string             `json:"container_id" db:"container_id"`
	ScheduleID  *string             `json:"schedule_id,omitempty" db:"schedule_id"`
	Artifacts   []ExecutionArtifact `json:"artifacts,omitempty"`
	StartedAt   *time.Time          `json:"started_at" db:"started_at"`
	CompletedAt *time.Time          `json:"completed_at" db:"completed_at"`
	CreatedAt   time.Time           `json:"created_at" db:"created_at"`
}

// ExecutionArtifact represents a file artifact produced by a command execution.
type ExecutionArtifact struct {
	ID            string    `json:"id" db:"id"`
	ExecutionID   string    `json:"execution_id" db:"execution_id"`
	Label         string    `json:"label" db:"label"`
	FileName      string    `json:"file_name" db:"file_name"`
	FileSizeBytes int64     `json:"file_size_bytes" db:"file_size_bytes"`
	ContentType   string    `json:"content_type" db:"content_type"`
	StoragePath   string    `json:"storage_path" db:"storage_path"`
	StorageType   string    `json:"storage_type" db:"storage_type"`
	Status        string    `json:"status" db:"status"`
	ErrorMessage  string    `json:"error_message,omitempty" db:"error_message"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
}

// OutputChunk represents a piece of output from a running command.
type OutputChunk struct {
	Stream    string    `json:"stream"`
	Data      string    `json:"data"`
	Timestamp time.Time `json:"timestamp"`
}

// ContainerConfig holds the configuration for creating a Docker container.
type ContainerConfig struct {
	Image          string            `json:"image"`
	Command        []string          `json:"command"`
	Volumes        []VolumeMount     `json:"volumes"`
	ResourceLimits ResourceLimits    `json:"resource_limits"`
	Environment    map[string]string `json:"environment,omitempty"`
}

// ExecInspectResult holds the result of inspecting a Docker exec instance.
type ExecInspectResult struct {
	ExitCode int  `json:"exit_code"`
	Running  bool `json:"running"`
}

// ExecutionFilter defines filtering and pagination options for listing executions.
type ExecutionFilter struct {
	CommandName string          `json:"command_name,omitempty"`
	UserID      string          `json:"user_id,omitempty"`
	DateFrom    *time.Time      `json:"date_from,omitempty"`
	DateTo      *time.Time      `json:"date_to,omitempty"`
	Status      ExecutionStatus `json:"status,omitempty"`
	Page        int             `json:"page"`
	PageSize    int             `json:"page_size"`
}
