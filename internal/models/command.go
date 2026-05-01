package models

import (
	"encoding/json"
	"time"
)

// ExecutionMode defines how a command is executed on Docker.
type ExecutionMode string

const (
	ModeCreate ExecutionMode = "create"
	ModeExec   ExecutionMode = "exec"
)

// ParameterType defines the data type of a command parameter.
type ParameterType string

const (
	ParamString  ParameterType = "string"
	ParamNumber  ParameterType = "number"
	ParamBoolean ParameterType = "boolean"
	ParamEnum    ParameterType = "enum"
)

// ArtifactDestType defines the storage backend for artifacts.
type ArtifactDestType string

const (
	DestLocal ArtifactDestType = "local"
	DestMinIO ArtifactDestType = "minio"
	DestS3    ArtifactDestType = "s3"
)

// CommandEntry represents a registered command in the catalog.
type CommandEntry struct {
	ID                  string              `json:"id" db:"id"`
	Name                string              `json:"name" db:"name"`
	Description         string              `json:"description" db:"description"`
	Category            string              `json:"category" db:"category"`
	DockerImage         string              `json:"docker_image" db:"docker_image"`
	CommandString       string              `json:"command_string" db:"command_string"`
	ParameterSchema     ParameterSchema     `json:"parameter_schema" db:"parameter_schema"`
	AllowedRoles        []Role              `json:"allowed_roles"`
	ResourceLimits      ResourceLimits      `json:"resource_limits" db:"resource_limits"`
	Volumes             []VolumeMount       `json:"volumes" db:"volumes"`
	TimeoutSeconds      int                 `json:"timeout_seconds" db:"timeout_seconds"`
	AllowConcurrent     bool                `json:"allow_concurrent" db:"allow_concurrent"`
	ExecutionMode       ExecutionMode       `json:"execution_mode" db:"execution_mode"`
	TargetContainer     string              `json:"target_container,omitempty" db:"target_container"`
	Artifacts           []ArtifactDeclare   `json:"artifacts,omitempty" db:"artifacts"`
	ArtifactDestination *ArtifactDestConfig `json:"artifact_destination,omitempty" db:"artifact_destination"`
	IsActive            bool                `json:"is_active" db:"is_active"`
	Version             int                 `json:"version" db:"version"`
	CreatedAt           time.Time           `json:"created_at" db:"created_at"`
	UpdatedAt           time.Time           `json:"updated_at" db:"updated_at"`
}

// ParameterSchema defines the parameters accepted by a command.
type ParameterSchema struct {
	Parameters []ParameterDefinition `json:"parameters"`
}

// Scan implements the sql.Scanner interface for reading JSONB from the database.
func (ps *ParameterSchema) Scan(src interface{}) error {
	if src == nil {
		ps.Parameters = []ParameterDefinition{}
		return nil
	}
	var data []byte
	switch v := src.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return nil
	}
	return json.Unmarshal(data, ps)
}

// Value implements the driver.Valuer interface for writing JSONB to the database.
func (ps ParameterSchema) Value() (interface{}, error) {
	return json.Marshal(ps)
}

// ParameterDefinition describes a single parameter in a command's schema.
type ParameterDefinition struct {
	Name          string          `json:"name"`
	Type          ParameterType   `json:"type"`
	Required      bool            `json:"required"`
	DefaultValue  interface{}     `json:"default_value,omitempty"`
	Description   string          `json:"description"`
	Validation    *ValidationRules `json:"validation,omitempty"`
	EnvVarMapping *EnvVarMapping  `json:"env_var_mapping,omitempty"`
}

// EnvVarMapping binds a parameter to an environment variable from a running sibling container.
type EnvVarMapping struct {
	SourceContainer string `json:"source_container"`
	EnvVarName      string `json:"env_var_name"`
}

// ValidationRules defines constraints for parameter validation.
type ValidationRules struct {
	Pattern    string   `json:"pattern,omitempty"`
	Min        *float64 `json:"min,omitempty"`
	Max        *float64 `json:"max,omitempty"`
	EnumValues []string `json:"enum_values,omitempty"`
	MaxLength  *int     `json:"max_length,omitempty"`
}

// ResourceLimits defines CPU and memory constraints for a container.
type ResourceLimits struct {
	CPUShares *int64 `json:"cpu_shares,omitempty"`
	MemoryMB  *int64 `json:"memory_mb,omitempty"`
	CPUCount  *int64 `json:"cpu_count,omitempty"`
}

// Scan implements the sql.Scanner interface for reading JSONB from the database.
func (rl *ResourceLimits) Scan(src interface{}) error {
	if src == nil {
		return nil
	}
	var data []byte
	switch v := src.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return nil
	}
	return json.Unmarshal(data, rl)
}

// Value implements the driver.Valuer interface for writing JSONB to the database.
func (rl ResourceLimits) Value() (interface{}, error) {
	return json.Marshal(rl)
}

// VolumeMount defines a host-to-container volume binding.
type VolumeMount struct {
	HostPath      string `json:"host_path"`
	ContainerPath string `json:"container_path"`
	ReadOnly      bool   `json:"read_only"`
}

// ArtifactDeclare describes a file artifact produced by a command execution.
type ArtifactDeclare struct {
	ContainerPath string `json:"container_path"`
	Label         string `json:"label"`
}

// ArtifactDestConfig defines the storage backend configuration for artifacts.
type ArtifactDestConfig struct {
	Type          ArtifactDestType `json:"type"`
	Bucket        string           `json:"bucket,omitempty"`
	PathPrefix    string           `json:"path_prefix,omitempty"`
	Endpoint      string           `json:"endpoint,omitempty"`
	CredentialRef string           `json:"credential_ref,omitempty"`
	Region        string           `json:"region,omitempty"`
}

// Scan implements the sql.Scanner interface for reading JSONB from the database.
func (adc *ArtifactDestConfig) Scan(src interface{}) error {
	if src == nil {
		return nil
	}
	var data []byte
	switch v := src.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return nil
	}
	return json.Unmarshal(data, adc)
}

// Value implements the driver.Valuer interface for writing JSONB to the database.
func (adc ArtifactDestConfig) Value() (interface{}, error) {
	return json.Marshal(adc)
}

// CommandFilter defines filtering and pagination options for listing commands.
type CommandFilter struct {
	Category string `json:"category,omitempty"`
	IsActive *bool  `json:"is_active,omitempty"`
	Roles    []Role `json:"roles,omitempty"`
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
	Search   string `json:"search,omitempty"`
}

// CreateCommandInput holds the data required to register a new command.
type CreateCommandInput struct {
	Name                string              `json:"name" binding:"required"`
	Description         string              `json:"description"`
	Category            string              `json:"category"`
	DockerImage         string              `json:"docker_image"`
	CommandString       string              `json:"command_string" binding:"required"`
	ParameterSchema     ParameterSchema     `json:"parameter_schema"`
	AllowedRoles        []Role              `json:"allowed_roles" binding:"required"`
	ResourceLimits      ResourceLimits      `json:"resource_limits"`
	Volumes             []VolumeMount       `json:"volumes"`
	TimeoutSeconds      int                 `json:"timeout_seconds"`
	AllowConcurrent     bool                `json:"allow_concurrent"`
	ExecutionMode       ExecutionMode       `json:"execution_mode"`
	TargetContainer     string              `json:"target_container,omitempty"`
	Artifacts           []ArtifactDeclare   `json:"artifacts,omitempty"`
	ArtifactDestination *ArtifactDestConfig `json:"artifact_destination,omitempty"`
}

// UpdateCommandInput holds the data for updating an existing command.
type UpdateCommandInput struct {
	Name                *string             `json:"name,omitempty"`
	Description         *string             `json:"description,omitempty"`
	Category            *string             `json:"category,omitempty"`
	DockerImage         *string             `json:"docker_image,omitempty"`
	CommandString       *string             `json:"command_string,omitempty"`
	ParameterSchema     *ParameterSchema    `json:"parameter_schema,omitempty"`
	AllowedRoles        []Role              `json:"allowed_roles,omitempty"`
	ResourceLimits      *ResourceLimits     `json:"resource_limits,omitempty"`
	Volumes             []VolumeMount       `json:"volumes,omitempty"`
	TimeoutSeconds      *int                `json:"timeout_seconds,omitempty"`
	AllowConcurrent     *bool               `json:"allow_concurrent,omitempty"`
	ExecutionMode       *ExecutionMode      `json:"execution_mode,omitempty"`
	TargetContainer     *string             `json:"target_container,omitempty"`
	Artifacts           []ArtifactDeclare   `json:"artifacts,omitempty"`
	ArtifactDestination *ArtifactDestConfig `json:"artifact_destination,omitempty"`
}

// PaginatedResult wraps a paginated list of items with metadata.
type PaginatedResult struct {
	Items    interface{} `json:"items"`
	Total    int         `json:"total"`
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
}
