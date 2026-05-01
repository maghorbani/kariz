CREATE TABLE execution_artifacts (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    execution_id UUID NOT NULL REFERENCES execution_records(id) ON DELETE CASCADE,
    label VARCHAR(255) NOT NULL,
    file_name VARCHAR(512) NOT NULL,
    file_size_bytes BIGINT NOT NULL DEFAULT 0,
    content_type VARCHAR(255) NOT NULL DEFAULT 'application/octet-stream',
    storage_path TEXT NOT NULL,
    storage_type VARCHAR(20) NOT NULL DEFAULT 'local',
    status VARCHAR(20) NOT NULL DEFAULT 'stored',
    error_message TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_execution_artifacts_execution_id ON execution_artifacts (execution_id);
