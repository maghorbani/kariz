package property_test

// Feature: kariz-command-dashboard, Property 8: parameter validation rejects invalid inputs
// Feature: kariz-command-dashboard, Property 14: shell injection prevention

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kariz/kariz/internal/models"
	"github.com/kariz/kariz/internal/validator"
	"pgregory.net/rapid"
)

// --- Helpers ---

// genParamType generates a random ParameterType.
func genParamType() *rapid.Generator[models.ParameterType] {
	return rapid.SampledFrom([]models.ParameterType{
		models.ParamString,
		models.ParamNumber,
		models.ParamBoolean,
		models.ParamEnum,
	})
}

// genParamName generates a valid parameter name (lowercase letters + underscores, 1-20 chars).
func genParamName() *rapid.Generator[string] {
	return rapid.StringMatching(`^[a-z][a-z0-9_]{0,19}$`)
}

// genEnumValues generates a list of 2-5 unique enum values.
func genEnumValues(t *rapid.T) []string {
	count := rapid.IntRange(2, 5).Draw(t, "enumCount")
	values := make([]string, count)
	seen := make(map[string]bool)
	for i := 0; i < count; i++ {
		for {
			v := rapid.StringMatching(`^[a-z]{2,10}$`).Draw(t, fmt.Sprintf("enumVal%d", i))
			if !seen[v] {
				seen[v] = true
				values[i] = v
				break
			}
		}
	}
	return values
}

// genParameterSchema generates a random ParameterSchema with 1-5 required parameters.
func genParameterSchema(t *rapid.T) models.ParameterSchema {
	count := rapid.IntRange(1, 5).Draw(t, "paramCount")
	params := make([]models.ParameterDefinition, count)
	usedNames := make(map[string]bool)

	for i := 0; i < count; i++ {
		// Generate a unique name.
		var name string
		for {
			name = genParamName().Draw(t, fmt.Sprintf("paramName%d", i))
			if !usedNames[name] {
				usedNames[name] = true
				break
			}
		}

		pType := genParamType().Draw(t, fmt.Sprintf("paramType%d", i))

		def := models.ParameterDefinition{
			Name:     name,
			Type:     pType,
			Required: true,
		}

		// Add validation rules based on type.
		switch pType {
		case models.ParamNumber:
			minVal := rapid.Float64Range(-1000, 0).Draw(t, fmt.Sprintf("min%d", i))
			maxVal := rapid.Float64Range(1, 1000).Draw(t, fmt.Sprintf("max%d", i))
			def.Validation = &models.ValidationRules{
				Min: &minVal,
				Max: &maxVal,
			}
		case models.ParamEnum:
			def.Validation = &models.ValidationRules{
				EnumValues: genEnumValues(t),
			}
		}

		params[i] = def
	}

	return models.ParameterSchema{Parameters: params}
}

// genValidParamValue generates a valid value for a given parameter definition.
func genValidParamValue(t *rapid.T, def models.ParameterDefinition, label string) interface{} {
	switch def.Type {
	case models.ParamString:
		return rapid.StringMatching(`^[a-zA-Z0-9_]{1,20}$`).Draw(t, label)
	case models.ParamNumber:
		if def.Validation != nil && def.Validation.Min != nil && def.Validation.Max != nil {
			return rapid.Float64Range(*def.Validation.Min, *def.Validation.Max).Draw(t, label)
		}
		return rapid.Float64Range(-100, 100).Draw(t, label)
	case models.ParamBoolean:
		return rapid.Bool().Draw(t, label)
	case models.ParamEnum:
		if def.Validation != nil && len(def.Validation.EnumValues) > 0 {
			return rapid.SampledFrom(def.Validation.EnumValues).Draw(t, label)
		}
		return rapid.StringMatching(`^[a-z]{2,10}$`).Draw(t, label)
	default:
		return "fallback"
	}
}

// genValidParams generates a complete valid parameter map for a schema.
func genValidParams(t *rapid.T, schema models.ParameterSchema) map[string]interface{} {
	params := make(map[string]interface{}, len(schema.Parameters))
	for i, def := range schema.Parameters {
		params[def.Name] = genValidParamValue(t, def, fmt.Sprintf("validVal%d", i))
	}
	return params
}

// --- Property 8: Parameter validation rejects invalid inputs ---
// **Validates: Requirements 8.3, 8.4**

// TestProperty8_MissingRequiredParam tests that removing a required parameter
// from a valid parameter map causes validation to fail with an error identifying
// the missing parameter by name.
func TestProperty8_MissingRequiredParam(t *testing.T) {
	v := validator.NewParameterValidator()

	rapid.Check(t, func(t *rapid.T) {
		schema := genParameterSchema(t)
		params := genValidParams(t, schema)

		// Pick a random required parameter to remove.
		idx := rapid.IntRange(0, len(schema.Parameters)-1).Draw(t, "removeIdx")
		removedName := schema.Parameters[idx].Name
		delete(params, removedName)

		result := v.Validate(schema, params)

		if result.Valid {
			t.Fatalf("expected validation to fail when required param %q is missing", removedName)
		}
		if len(result.Errors) == 0 {
			t.Fatal("expected at least one validation error")
		}

		// At least one error should reference the removed parameter.
		found := false
		for _, e := range result.Errors {
			if e.ParameterName == removedName {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected error for parameter %q, got errors: %v", removedName, result.Errors)
		}
	})
}

// TestProperty8_WrongType tests that providing a value of the wrong type
// causes validation to fail with an error identifying the offending parameter.
func TestProperty8_WrongType(t *testing.T) {
	v := validator.NewParameterValidator()

	rapid.Check(t, func(t *rapid.T) {
		schema := genParameterSchema(t)
		params := genValidParams(t, schema)

		// Pick a random parameter to give a wrong-type value.
		idx := rapid.IntRange(0, len(schema.Parameters)-1).Draw(t, "wrongTypeIdx")
		targetParam := schema.Parameters[idx]

		// Provide a value of a different type.
		switch targetParam.Type {
		case models.ParamString:
			params[targetParam.Name] = float64(999) // number instead of string
		case models.ParamNumber:
			params[targetParam.Name] = "not-a-number" // string instead of number
		case models.ParamBoolean:
			params[targetParam.Name] = "not-a-bool" // string instead of bool
		case models.ParamEnum:
			params[targetParam.Name] = float64(42) // number instead of string
		}

		result := v.Validate(schema, params)

		if result.Valid {
			t.Fatalf("expected validation to fail for wrong type on param %q", targetParam.Name)
		}
		if len(result.Errors) == 0 {
			t.Fatal("expected at least one validation error")
		}

		found := false
		for _, e := range result.Errors {
			if e.ParameterName == targetParam.Name {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected error for parameter %q, got errors: %v", targetParam.Name, result.Errors)
		}
	})
}

// TestProperty8_UnknownParameter tests that providing a parameter not in the
// schema causes validation to fail with an error identifying the unknown parameter.
func TestProperty8_UnknownParameter(t *testing.T) {
	v := validator.NewParameterValidator()

	rapid.Check(t, func(t *rapid.T) {
		schema := genParameterSchema(t)
		params := genValidParams(t, schema)

		// Generate a parameter name that is NOT in the schema.
		knownNames := make(map[string]bool)
		for _, p := range schema.Parameters {
			knownNames[p.Name] = true
		}
		var unknownName string
		for {
			unknownName = rapid.StringMatching(`^unknown_[a-z]{3,8}$`).Draw(t, "unknownName")
			if !knownNames[unknownName] {
				break
			}
		}
		params[unknownName] = "some_value"

		result := v.Validate(schema, params)

		if result.Valid {
			t.Fatalf("expected validation to fail for unknown param %q", unknownName)
		}
		if len(result.Errors) == 0 {
			t.Fatal("expected at least one validation error")
		}

		found := false
		for _, e := range result.Errors {
			if e.ParameterName == unknownName && e.Code == "unknown_parameter" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected unknown_parameter error for %q, got errors: %v", unknownName, result.Errors)
		}
	})
}

// TestProperty8_NumberOutsideRange tests that providing a number outside the
// min/max range causes validation to fail.
func TestProperty8_NumberOutsideRange(t *testing.T) {
	v := validator.NewParameterValidator()

	rapid.Check(t, func(t *rapid.T) {
		// Generate a schema with at least one number parameter with min/max.
		minVal := rapid.Float64Range(0, 100).Draw(t, "min")
		maxVal := minVal + rapid.Float64Range(1, 100).Draw(t, "range")

		schema := models.ParameterSchema{
			Parameters: []models.ParameterDefinition{
				{
					Name:     "num_param",
					Type:     models.ParamNumber,
					Required: true,
					Validation: &models.ValidationRules{
						Min: &minVal,
						Max: &maxVal,
					},
				},
			},
		}

		// Decide whether to go below min or above max.
		goBelow := rapid.Bool().Draw(t, "goBelow")
		var badValue float64
		if goBelow {
			badValue = minVal - rapid.Float64Range(0.01, 1000).Draw(t, "belowOffset")
		} else {
			badValue = maxVal + rapid.Float64Range(0.01, 1000).Draw(t, "aboveOffset")
		}

		params := map[string]interface{}{
			"num_param": badValue,
		}

		result := v.Validate(schema, params)

		if result.Valid {
			t.Fatalf("expected validation to fail for value %v outside range [%v, %v]", badValue, minVal, maxVal)
		}
		if len(result.Errors) == 0 {
			t.Fatal("expected at least one validation error")
		}

		found := false
		for _, e := range result.Errors {
			if e.ParameterName == "num_param" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected error for parameter 'num_param', got errors: %v", result.Errors)
		}
	})
}

// TestProperty8_EnumInvalidValue tests that providing a value not in the enum
// list causes validation to fail.
func TestProperty8_EnumInvalidValue(t *testing.T) {
	v := validator.NewParameterValidator()

	rapid.Check(t, func(t *rapid.T) {
		enumValues := genEnumValues(t)

		schema := models.ParameterSchema{
			Parameters: []models.ParameterDefinition{
				{
					Name:     "enum_param",
					Type:     models.ParamEnum,
					Required: true,
					Validation: &models.ValidationRules{
						EnumValues: enumValues,
					},
				},
			},
		}

		// Generate a value that is NOT in the enum list.
		enumSet := make(map[string]bool)
		for _, v := range enumValues {
			enumSet[v] = true
		}
		var badValue string
		for {
			badValue = rapid.StringMatching(`^[a-z]{2,15}$`).Draw(t, "badEnum")
			if !enumSet[badValue] {
				break
			}
		}

		params := map[string]interface{}{
			"enum_param": badValue,
		}

		result := v.Validate(schema, params)

		if result.Valid {
			t.Fatalf("expected validation to fail for enum value %q not in %v", badValue, enumValues)
		}
		if len(result.Errors) == 0 {
			t.Fatal("expected at least one validation error")
		}

		found := false
		for _, e := range result.Errors {
			if e.ParameterName == "enum_param" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected error for parameter 'enum_param', got errors: %v", result.Errors)
		}
	})
}

// --- Property 14: Shell injection prevention ---
// **Validates: Requirements 8.3**

// shellMetachars contains shell metacharacters that could be used for injection.
var shellMetachars = []string{";", "|", "$(", ")", "`", "&&", "||", ">", "<", "\n", "&", ">>", "<<", "$((", "${", "}", "'", "\"", "\\"}

// genShellInjectionValue generates a string containing shell metacharacters.
func genShellInjectionValue(t *rapid.T, label string) string {
	// Build a string with random shell metacharacters mixed in.
	prefix := rapid.StringMatching(`^[a-zA-Z0-9]{0,10}$`).Draw(t, label+"_prefix")
	metaCount := rapid.IntRange(1, 3).Draw(t, label+"_metaCount")
	var parts []string
	parts = append(parts, prefix)
	for i := 0; i < metaCount; i++ {
		meta := rapid.SampledFrom(shellMetachars).Draw(t, fmt.Sprintf("%s_meta%d", label, i))
		word := rapid.StringMatching(`^[a-zA-Z0-9/_ ]{0,10}$`).Draw(t, fmt.Sprintf("%s_word%d", label, i))
		parts = append(parts, meta+word)
	}
	return strings.Join(parts, "")
}

// TestProperty14_ShellInjectionPrevention tests that parameter values containing
// shell metacharacters appear as exactly one isolated element in the output args array.
func TestProperty14_ShellInjectionPrevention(t *testing.T) {
	v := validator.NewParameterValidator()

	rapid.Check(t, func(t *rapid.T) {
		// Generate 1-3 parameter placeholders.
		paramCount := rapid.IntRange(1, 3).Draw(t, "paramCount")
		paramDefs := make([]models.ParameterDefinition, paramCount)
		paramNames := make([]string, paramCount)
		usedNames := make(map[string]bool)

		for i := 0; i < paramCount; i++ {
			var name string
			for {
				name = rapid.StringMatching(`^[a-z][a-z0-9_]{1,10}$`).Draw(t, fmt.Sprintf("pname%d", i))
				if !usedNames[name] {
					usedNames[name] = true
					break
				}
			}
			paramNames[i] = name
			paramDefs[i] = models.ParameterDefinition{
				Name: name,
				Type: models.ParamString,
			}
		}

		schema := models.ParameterSchema{Parameters: paramDefs}

		// Build a command template: "cmd" followed by placeholders.
		cmdParts := []string{"cmd"}
		for _, name := range paramNames {
			cmdParts = append(cmdParts, "{{"+name+"}}")
		}
		commandString := strings.Join(cmdParts, " ")

		// Generate parameter values with shell metacharacters.
		params := make(map[string]interface{}, paramCount)
		paramValues := make([]string, paramCount)
		for i, name := range paramNames {
			val := genShellInjectionValue(t, fmt.Sprintf("val%d", i))
			params[name] = val
			paramValues[i] = val
		}

		args, err := v.BuildCommandArgs(commandString, schema, params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Expected: ["cmd", val0, val1, ...] — each param value is exactly one element.
		expectedLen := 1 + paramCount
		if len(args) != expectedLen {
			t.Fatalf("expected %d args, got %d: %v (command: %q, params: %v)",
				expectedLen, len(args), args, commandString, params)
		}

		if args[0] != "cmd" {
			t.Fatalf("expected first arg to be 'cmd', got %q", args[0])
		}

		// Each parameter value should appear as exactly one isolated element.
		for i, val := range paramValues {
			if args[i+1] != val {
				t.Fatalf("expected args[%d] to be %q (exact param value), got %q",
					i+1, val, args[i+1])
			}
		}
	})
}

// TestProperty14_OutputArrayLength tests that the output array length matches
// the expected count of static tokens plus parameter tokens.
func TestProperty14_OutputArrayLength(t *testing.T) {
	v := validator.NewParameterValidator()

	rapid.Check(t, func(t *rapid.T) {
		// Generate 1-3 static tokens and 1-3 parameter placeholders.
		staticCount := rapid.IntRange(1, 3).Draw(t, "staticCount")
		paramCount := rapid.IntRange(1, 3).Draw(t, "paramCount")

		paramDefs := make([]models.ParameterDefinition, paramCount)
		usedNames := make(map[string]bool)
		paramNames := make([]string, paramCount)

		for i := 0; i < paramCount; i++ {
			var name string
			for {
				name = rapid.StringMatching(`^[a-z][a-z0-9_]{1,10}$`).Draw(t, fmt.Sprintf("pn%d", i))
				if !usedNames[name] {
					usedNames[name] = true
					break
				}
			}
			paramNames[i] = name
			paramDefs[i] = models.ParameterDefinition{
				Name: name,
				Type: models.ParamString,
			}
		}

		schema := models.ParameterSchema{Parameters: paramDefs}

		// Build command: static tokens interspersed with placeholders.
		var cmdParts []string
		for i := 0; i < staticCount; i++ {
			token := rapid.StringMatching(`^[a-z]{2,8}$`).Draw(t, fmt.Sprintf("static%d", i))
			cmdParts = append(cmdParts, token)
		}
		for _, name := range paramNames {
			cmdParts = append(cmdParts, "{{"+name+"}}")
		}
		commandString := strings.Join(cmdParts, " ")

		// Generate parameter values (with shell metacharacters).
		params := make(map[string]interface{}, paramCount)
		for _, name := range paramNames {
			params[name] = genShellInjectionValue(t, "injVal_"+name)
		}

		args, err := v.BuildCommandArgs(commandString, schema, params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expectedLen := staticCount + paramCount
		if len(args) != expectedLen {
			t.Fatalf("expected %d args (static=%d + params=%d), got %d: %v",
				expectedLen, staticCount, paramCount, len(args), args)
		}
	})
}
