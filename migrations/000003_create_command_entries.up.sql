CREATE TABLE command_entries (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    category VARCHAR(255) NOT NULL DEFAULT '',
    docker_image VARCHAR(512) NOT NULL DEFAULT '',
    command_string TEXT NOT NULL,
    parameter_schema JSONB NOT NULL DEFAULT '{"parameters":[]}',
    resource_limits JSONB NOT NULL DEFAULT '{}',
    volumes JSONB NOT NULL DEFAULT '[]',
    timeout_seconds INTEGER NOT NULL DEFAULT 300,
    allow_concurrent BOOLEAN NOT NULL DEFAULT FALSE,
    execution_mode VARCHAR(20) NOT NULL DEFAULT 'create',
    target_container VARCHAR(255),
    artifacts JSONB NOT NULL DEFAULT '[]',
    artifact_destination JSONB,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    version INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_command_entries_name ON command_entries (name);
CREATE INDEX idx_command_entries_category ON command_entries (category);
CREATE INDEX idx_command_entries_is_active ON command_entries (is_active);
