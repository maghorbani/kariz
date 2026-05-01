package models

// APIError represents a structured error response returned by the API.
type APIError struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Details []ValidationError `json:"details,omitempty"`
}

// Error implements the error interface for APIError.
func (e *APIError) Error() string {
	return e.Message
}

// ValidationResult holds the outcome of parameter validation.
type ValidationResult struct {
	Valid  bool              `json:"valid"`
	Errors []ValidationError `json:"errors,omitempty"`
}

// ValidationError describes a single validation failure for a parameter.
type ValidationError struct {
	ParameterName string `json:"parameter_name"`
	Message       string `json:"message"`
	Code          string `json:"code"`
}

// EnvVarResolutionError provides details when environment variable resolution fails.
type EnvVarResolutionError struct {
	ParameterName   string `json:"parameter_name"`
	SourceContainer string `json:"source_container"`
	EnvVarName      string `json:"env_var_name"`
	Reason          string `json:"reason"`
}

// Error implements the error interface for EnvVarResolutionError.
func (e *EnvVarResolutionError) Error() string {
	return "env var resolution failed for parameter " + e.ParameterName + ": " + e.Reason
}
