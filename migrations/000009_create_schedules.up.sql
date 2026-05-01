CREATE TABLE schedules (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    command_id UUID NOT NULL REFERENCES command_entries(id) ON DELETE CASCADE,
    created_by_user UUID NOT NULL REFERENCES users(id),
    command_name VARCHAR(255) NOT NULL,
    cron_expression VARCHAR(255),
    interval_seconds INTEGER,
    parameters JSONB NOT NULL DEFAULT '{}',
    is_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    next_run_at TIMESTAMP WITH TIME ZONE,
    last_run_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_schedules_command_id ON schedules (command_id);
CREATE INDEX idx_schedules_created_by_user ON schedules (created_by_user);
CREATE INDEX idx_schedules_next_run_at ON schedules (next_run_at)
    WHERE is_enabled = TRUE;
