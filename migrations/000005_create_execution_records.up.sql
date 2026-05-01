CREATE TABLE execution_records (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    command_id UUID NOT NULL REFERENCES command_entries(id),
    user_id UUID NOT NULL REFERENCES users(id),
    schedule_id UUID,
    command_name VARCHAR(255) NOT NULL,
    parameters JSONB NOT NULL DEFAULT '{}',
    status VARCHAR(20) NOT NULL DEFAULT 'queued',
    exit_code INTEGER,
    stdout TEXT NOT NULL DEFAULT '',
    stderr TEXT NOT NULL DEFAULT '',
    container_id VARCHAR(255),
    started_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_execution_records_command_id ON execution_records (command_id);
CREATE INDEX idx_execution_records_user_id ON execution_records (user_id);
CREATE INDEX idx_execution_records_status ON execution_records (status);
CREATE INDEX idx_execution_records_created_at ON execution_records (created_at DESC);
CREATE INDEX idx_execution_records_schedule_id ON execution_records (schedule_id);
