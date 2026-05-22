// User and auth types

export interface UserProfile {
  id: string;
  username: string;
  email: string;
  roles: string[];
  is_active: boolean;
}

export interface CreateUserRequest {
  username: string;
  password: string;
  email?: string;
  roles: string[];
}

export interface UpdateUserRequest {
  email?: string;
  is_active?: boolean;
  password?: string;
}

export interface LoginCredentials {
  username: string;
  password: string;
}

export interface ApiErrorBody {
  code: string;
  message: string;
}

// Command catalog types

export interface ParameterDefinition {
  name: string;
  type: 'string' | 'number' | 'boolean' | 'enum';
  required: boolean;
  default_value?: unknown;
  description: string;
  validation?: ValidationRules;
  env_var_mapping?: EnvVarMapping;
}

export interface EnvVarMapping {
  source_container: string;
  env_var_name: string;
}

export interface ValidationRules {
  pattern?: string;
  min?: number;
  max?: number;
  enum_values?: string[];
  max_length?: number;
}

export interface ParameterSchema {
  parameters: ParameterDefinition[];
}

export interface ResourceLimits {
  cpu_shares?: number;
  memory_mb?: number;
  cpu_count?: number;
}

export interface VolumeMount {
  host_path: string;
  container_path: string;
  read_only: boolean;
}

export interface ArtifactDeclare {
  container_path: string;
  label: string;
}

export interface ArtifactDestConfig {
  type: 'local' | 'minio' | 's3';
  bucket?: string;
  path_prefix?: string;
  endpoint?: string;
  credential_ref?: string;
  region?: string;
}

export interface LogOptions {
  tail_lines: number;
  follow: boolean;
  timestamps: boolean;
}

export interface CommandEntry {
  id: string;
  name: string;
  description: string;
  category: string;
  docker_image: string;
  command_string: string;
  parameter_schema: ParameterSchema;
  allowed_roles: string[];
  resource_limits: ResourceLimits;
  volumes: VolumeMount[];
  timeout_seconds: number;
  allow_concurrent: boolean;
  execution_mode: 'create' | 'exec' | 'logs';
  target_container?: string;
  log_options?: LogOptions;
  artifacts?: ArtifactDeclare[];
  artifact_destination?: ArtifactDestConfig;
  is_active: boolean;
  version: number;
  created_at: string;
  updated_at: string;
}

// Execution types

export type ExecutionStatus =
  | 'queued'
  | 'running'
  | 'completed'
  | 'failed'
  | 'timed_out'
  | 'cancelled';

export interface ExecutionArtifact {
  id: string;
  execution_id: string;
  label: string;
  file_name: string;
  file_size_bytes: number;
  content_type: string;
  storage_path: string;
  storage_type: string;
  status: string;
  error_message?: string;
  created_at: string;
}

export interface ExecutionRecord {
  id: string;
  command_id: string;
  command_name: string;
  user_id: string;
  parameters: Record<string, unknown>;
  status: ExecutionStatus;
  exit_code?: number;
  stdout: string;
  stderr: string;
  container_id?: string;
  schedule_id?: string;
  artifacts?: ExecutionArtifact[];
  started_at?: string;
  completed_at?: string;
  created_at: string;
}

// Notification types

export type NotificationType =
  | 'execution_complete'
  | 'execution_failed'
  | 'execution_timed_out';

export interface Notification {
  id: string;
  user_id: string;
  type: NotificationType;
  title: string;
  message: string;
  execution_id: string;
  is_read: boolean;
  created_at: string;
}

// Schedule types

export interface Schedule {
  id: string;
  command_id: string;
  command_name: string;
  created_by_user: string;
  cron_expression?: string;
  interval_seconds?: number;
  parameters?: Record<string, unknown>;
  is_enabled: boolean;
  next_run_at?: string;
  last_run_at?: string;
  created_at: string;
  updated_at: string;
}

// Pagination

export interface PaginatedResult<T> {
  items: T[];
  total: number;
  page: number;
  page_size: number;
}
