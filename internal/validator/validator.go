package validator

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/kariz/kariz/internal/models"
)

// ParameterValidator validates user-provided parameters against a command's
// ParameterSchema and performs safe substitution into command strings.
type ParameterValidator interface {
	Validate(schema models.ParameterSchema, params map[string]interface{}) *models.ValidationResult
	BuildCommandArgs(commandString string, schema models.ParameterSchema, validatedParams map[string]interface{}) ([]string, error)
}

// paramValidator is the default implementation of ParameterValidator.
type paramValidator struct{}

// NewParameterValidator creates a new ParameterValidator instance.
func NewParameterValidator() ParameterValidator {
	return &paramValidator{}
}

// Validate checks user-provided parameters against the schema.
// It detects unknown parameters, checks required fields, validates types,
// and enforces constraints (pattern, min/max, enum values, max length).
func (v *paramValidator) Validate(schema models.ParameterSchema, params map[string]interface{}) *models.ValidationResult {
	result := &models.ValidationResult{
		Valid:  true,
		Errors: []models.ValidationError{},
	}

	// Build a set of known parameter names from the schema.
	knownParams := make(map[string]bool, len(schema.Parameters))
	for _, p := range schema.Parameters {
		knownParams[p.Name] = true
	}

	// Detect unknown parameters.
	for name := range params {
		if !knownParams[name] {
			result.Valid = false
			result.Errors = append(result.Errors, models.ValidationError{
				ParameterName: name,
				Message:       fmt.Sprintf("unknown parameter %q", name),
				Code:          "unknown_parameter",
			})
		}
	}

	// Validate each parameter defined in the schema.
	for _, paramDef := range schema.Parameters {
		value, provided := params[paramDef.Name]

		if !provided || value == nil {
			if paramDef.Required && paramDef.DefaultValue == nil {
				result.Valid = false
				result.Errors = append(result.Errors, models.ValidationError{
					ParameterName: paramDef.Name,
					Message:       fmt.Sprintf("parameter %q is required", paramDef.Name),
					Code:          "required",
				})
			}
			// If not provided but has a default, the caller can apply the default.
			// No validation error needed.
			continue
		}

		// Type validation and constraint validation.
		v.validateType(paramDef, value, result)
	}

	return result
}

// validateType checks that the value matches the expected type and constraints.
func (v *paramValidator) validateType(paramDef models.ParameterDefinition, value interface{}, result *models.ValidationResult) {
	switch paramDef.Type {
	case models.ParamString:
		strVal, ok := value.(string)
		if !ok {
			result.Valid = false
			result.Errors = append(result.Errors, models.ValidationError{
				ParameterName: paramDef.Name,
				Message:       fmt.Sprintf("parameter %q must be a string", paramDef.Name),
				Code:          "invalid_type",
			})
			return
		}
		v.validateStringConstraints(paramDef, strVal, result)

	case models.ParamNumber:
		numVal, ok := toFloat64(value)
		if !ok {
			result.Valid = false
			result.Errors = append(result.Errors, models.ValidationError{
				ParameterName: paramDef.Name,
				Message:       fmt.Sprintf("parameter %q must be a number", paramDef.Name),
				Code:          "invalid_type",
			})
			return
		}
		v.validateNumberConstraints(paramDef, numVal, result)

	case models.ParamBoolean:
		if _, ok := value.(bool); !ok {
			result.Valid = false
			result.Errors = append(result.Errors, models.ValidationError{
				ParameterName: paramDef.Name,
				Message:       fmt.Sprintf("parameter %q must be a boolean", paramDef.Name),
				Code:          "invalid_type",
			})
		}

	case models.ParamEnum:
		strVal, ok := value.(string)
		if !ok {
			result.Valid = false
			result.Errors = append(result.Errors, models.ValidationError{
				ParameterName: paramDef.Name,
				Message:       fmt.Sprintf("parameter %q must be a string for enum type", paramDef.Name),
				Code:          "invalid_type",
			})
			return
		}
		v.validateEnumConstraints(paramDef, strVal, result)

	default:
		result.Valid = false
		result.Errors = append(result.Errors, models.ValidationError{
			ParameterName: paramDef.Name,
			Message:       fmt.Sprintf("parameter %q has unsupported type %q", paramDef.Name, paramDef.Type),
			Code:          "invalid_type",
		})
	}
}

// validateStringConstraints checks pattern and max length constraints for string values.
func (v *paramValidator) validateStringConstraints(paramDef models.ParameterDefinition, value string, result *models.ValidationResult) {
	if paramDef.Validation == nil {
		return
	}

	if paramDef.Validation.Pattern != "" {
		re, err := regexp.Compile(paramDef.Validation.Pattern)
		if err != nil {
			result.Valid = false
			result.Errors = append(result.Errors, models.ValidationError{
				ParameterName: paramDef.Name,
				Message:       fmt.Sprintf("parameter %q has invalid validation pattern: %v", paramDef.Name, err),
				Code:          "validation_failed",
			})
			return
		}
		if !re.MatchString(value) {
			result.Valid = false
			result.Errors = append(result.Errors, models.ValidationError{
				ParameterName: paramDef.Name,
				Message:       fmt.Sprintf("parameter %q value %q does not match pattern %q", paramDef.Name, value, paramDef.Validation.Pattern),
				Code:          "validation_failed",
			})
		}
	}

	if paramDef.Validation.MaxLength != nil {
		if len(value) > *paramDef.Validation.MaxLength {
			result.Valid = false
			result.Errors = append(result.Errors, models.ValidationError{
				ParameterName: paramDef.Name,
				Message:       fmt.Sprintf("parameter %q exceeds maximum length of %d", paramDef.Name, *paramDef.Validation.MaxLength),
				Code:          "validation_failed",
			})
		}
	}
}

// validateNumberConstraints checks min and max constraints for number values.
func (v *paramValidator) validateNumberConstraints(paramDef models.ParameterDefinition, value float64, result *models.ValidationResult) {
	if paramDef.Validation == nil {
		return
	}

	if paramDef.Validation.Min != nil {
		if value < *paramDef.Validation.Min {
			result.Valid = false
			result.Errors = append(result.Errors, models.ValidationError{
				ParameterName: paramDef.Name,
				Message:       fmt.Sprintf("parameter %q value %v is less than minimum %v", paramDef.Name, value, *paramDef.Validation.Min),
				Code:          "validation_failed",
			})
		}
	}

	if paramDef.Validation.Max != nil {
		if value > *paramDef.Validation.Max {
			result.Valid = false
			result.Errors = append(result.Errors, models.ValidationError{
				ParameterName: paramDef.Name,
				Message:       fmt.Sprintf("parameter %q value %v exceeds maximum %v", paramDef.Name, value, *paramDef.Validation.Max),
				Code:          "validation_failed",
			})
		}
	}
}

// validateEnumConstraints checks that the value is in the allowed enum values list.
func (v *paramValidator) validateEnumConstraints(paramDef models.ParameterDefinition, value string, result *models.ValidationResult) {
	if paramDef.Validation == nil || len(paramDef.Validation.EnumValues) == 0 {
		return
	}

	for _, allowed := range paramDef.Validation.EnumValues {
		if value == allowed {
			return
		}
	}

	result.Valid = false
	result.Errors = append(result.Errors, models.ValidationError{
		ParameterName: paramDef.Name,
		Message:       fmt.Sprintf("parameter %q value %q is not one of the allowed values: %v", paramDef.Name, value, paramDef.Validation.EnumValues),
		Code:          "validation_failed",
	})
}

// placeholderRegex matches {{param_name}} placeholders in command strings.
var placeholderRegex = regexp.MustCompile(`\{\{(\w+)\}\}`)

// BuildCommandArgs parses a command string template and substitutes validated
// parameters as isolated array elements. Each parameter value becomes a single
// element in the returned slice — never concatenated into a shell string.
// This prevents shell injection because the Docker exec API takes []string.
func (v *paramValidator) BuildCommandArgs(commandString string, schema models.ParameterSchema, validatedParams map[string]interface{}) ([]string, error) {
	// Split the command string by whitespace to get tokens.
	tokens := strings.Fields(commandString)
	args := make([]string, 0, len(tokens))

	for _, token := range tokens {
		// Check if the entire token is a placeholder like {{param_name}}.
		if placeholderRegex.MatchString(token) {
			// Replace all placeholders in this token.
			replaced := placeholderRegex.ReplaceAllStringFunc(token, func(match string) string {
				// Extract the parameter name from {{name}}.
				paramName := match[2 : len(match)-2]
				if val, ok := validatedParams[paramName]; ok {
					return fmt.Sprintf("%v", val)
				}
				// If not found in validated params, check for defaults in schema.
				for _, p := range schema.Parameters {
					if p.Name == paramName && p.DefaultValue != nil {
						return fmt.Sprintf("%v", p.DefaultValue)
					}
				}
				// Leave the placeholder as-is if no value found.
				return match
			})
			args = append(args, replaced)
		} else {
			args = append(args, token)
		}
	}

	return args, nil
}

// toFloat64 attempts to convert a value to float64.
// Handles float64 (JSON default), int, int64, float32, and other numeric types.
func toFloat64(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int8:
		return float64(n), true
	case int16:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint8:
		return float64(n), true
	case uint16:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		return float64(n), true
	default:
		return 0, false
	}
}
