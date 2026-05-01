CREATE TABLE command_roles (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    command_id UUID NOT NULL REFERENCES command_entries(id) ON DELETE CASCADE,
    role VARCHAR(50) NOT NULL,
    UNIQUE (command_id, role)
);

CREATE INDEX idx_command_roles_command_id ON command_roles (command_id);
CREATE INDEX idx_command_roles_role ON command_roles (role);
