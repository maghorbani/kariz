package models

import (
	"encoding/json"
	"time"
)

// Schedule defines a recurring or one-time trigger for automatic command execution.
type Schedule struct {
	ID             string          `json:"id" db:"id"`
	CommandID      string          `json:"command_id" db:"command_id"`
	CommandName    string          `json:"command_name" db:"command_name"`
	CreatedByUser  string          `json:"created_by_user" db:"created_by_user"`
	CronExpression string          `json:"cron_expression,omitempty" db:"cron_expression"`
	IntervalSec    *int            `json:"interval_seconds,omitempty" db:"interval_seconds"`
	Parameters     json.RawMessage `json:"parameters,omitempty" db:"parameters"`
	IsEnabled      bool            `json:"is_enabled" db:"is_enabled"`
	NextRunAt      *time.Time      `json:"next_run_at" db:"next_run_at"`
	LastRunAt      *time.Time      `json:"last_run_at" db:"last_run_at"`
	CreatedAt      time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at" db:"updated_at"`
}

// CreateScheduleInput holds the data required to create a new schedule.
type CreateScheduleInput struct {
	CommandID      string          `json:"command_id" binding:"required"`
	CronExpression string          `json:"cron_expression,omitempty"`
	IntervalSec    *int            `json:"interval_seconds,omitempty"`
	Parameters     json.RawMessage `json:"parameters,omitempty"`
}

// UpdateScheduleInput holds the data for updating an existing schedule.
type UpdateScheduleInput struct {
	CronExpression *string          `json:"cron_expression,omitempty"`
	IntervalSec    *int             `json:"interval_seconds,omitempty"`
	Parameters     *json.RawMessage `json:"parameters,omitempty"`
}

// ScheduleFilter defines filtering and pagination options for listing schedules.
type ScheduleFilter struct {
	CommandID string `json:"command_id,omitempty"`
	IsEnabled *bool  `json:"is_enabled,omitempty"`
	Roles     []Role `json:"roles,omitempty"`
	Page      int    `json:"page"`
	PageSize  int    `json:"page_size"`
}
