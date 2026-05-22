package catalog

import (
	"strings"

	"github.com/kariz/kariz/internal/models"
)

func validateCommandFields(
	mode models.ExecutionMode,
	commandString string,
	dockerImage string,
	targetContainer string,
	logOptions *models.LogOptions,
	timeoutSeconds int,
) error {
	if mode == "" {
		mode = models.ModeCreate
	}

	switch mode {
	case models.ModeLogs:
		if strings.TrimSpace(targetContainer) == "" {
			return &models.APIError{
				Code:    "validation_error",
				Message: "target_container is required for logs mode",
			}
		}
		if logOptions == nil {
			logOptions = models.DefaultLogOptions()
		}
		if logOptions.TailLines < 0 {
			return &models.APIError{
				Code:    "validation_error",
				Message: "log tail_lines must be non-negative",
			}
		}
	case models.ModeExec:
		if strings.TrimSpace(targetContainer) == "" {
			return &models.APIError{
				Code:    "validation_error",
				Message: "target_container is required for exec mode",
			}
		}
		if strings.TrimSpace(commandString) == "" {
			return &models.APIError{
				Code:    "validation_error",
				Message: "command_string is required",
			}
		}
	default:
		if strings.TrimSpace(commandString) == "" {
			return &models.APIError{
				Code:    "validation_error",
				Message: "command_string is required",
			}
		}
		if strings.TrimSpace(dockerImage) == "" {
			return &models.APIError{
				Code:    "validation_error",
				Message: "docker_image is required for create mode",
			}
		}
	}

	if timeoutSeconds < 0 {
		return &models.APIError{
			Code:    "validation_error",
			Message: "timeout_seconds cannot be negative",
		}
	}
	if timeoutSeconds > models.MaxTimeoutSeconds {
		return &models.APIError{
			Code:    "validation_error",
			Message: "timeout_seconds cannot exceed 86400 (24 hours)",
		}
	}

	return nil
}
