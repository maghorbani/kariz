package validator

import (
	"testing"

	"github.com/kariz/kariz/internal/models"
)

func float64Ptr(v float64) *float64 { return &v }
func intPtr(v int) *int             { return &v }

// --- Validate Tests ---

func TestValidate_ValidParams_Passes(t *testing.T) {
	schema := models.ParameterSchema{
		Parameters: []models.ParameterDefinition{
			{Name: "name", Type: models.ParamString, Required: true},
			{Name: "count", Type: models.ParamNumber, Required: true},
			{Name: "verbose", Type: models.ParamBoolean, Required: false},
		},
	}
	params := map[string]interface{}{
		"name":    "test",
		"count":   float64(5),
		"verbose": true,
	}

	v := NewParameterValidator()
	result := v.Validate(schema, params)

	if !result.Valid {
		t.Errorf("expected valid, got errors: %v", result.Errors)
	}
	if len(result.Errors) != 0 {
		t.Errorf("expected 0 errors, got %d", len(result.Errors))
	}
}

func TestValidate_MissingRequiredParam_Fails(t *testing.T) {
	schema := models.ParameterSchema{
		Parameters: []models.ParameterDefinition{
			{Name: "db_name", Type: models.ParamString, Required: true},
		},
	}
	params := map[string]interface{}{}

	v := NewParameterValidator()
	result := v.Validate(schema, params)

	if result.Valid {
		t.Fatal("expected invalid result")
	}
	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(result.Errors))
	}
	if result.Errors[0].Code != "required" {
		t.Errorf("expected code 'required', got %q", result.Errors[0].Code)
	}
	if result.Errors[0].ParameterName != "db_name" {
		t.Errorf("expected parameter name 'db_name', got %q", result.Errors[0].ParameterName)
	}
}

func TestValidate_MissingRequiredParamWithDefault_Passes(t *testing.T) {
	schema := models.ParameterSchema{
		Parameters: []models.ParameterDefinition{
			{Name: "db_name", Type: models.ParamString, Required: true, DefaultValue: "production"},
		},
	}
	params := map[string]interface{}{}

	v := NewParameterValidator()
	result := v.Validate(schema, params)

	if !result.Valid {
		t.Errorf("expected valid (default should satisfy required), got errors: %v", result.Errors)
	}
}

func TestValidate_WrongType_String_Fails(t *testing.T) {
	schema := models.ParameterSchema{
		Parameters: []models.ParameterDefinition{
			{Name: "name", Type: models.ParamString, Required: true},
		},
	}
	params := map[string]interface{}{
		"name": float64(42),
	}

	v := NewParameterValidator()
	result := v.Validate(schema, params)

	if result.Valid {
		t.Fatal("expected invalid result")
	}
	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(result.Errors))
	}
	if result.Errors[0].Code != "invalid_type" {
		t.Errorf("expected code 'invalid_type', got %q", result.Errors[0].Code)
	}
}

func TestValidate_WrongType_Number_Fails(t *testing.T) {
	schema := models.ParameterSchema{
		Parameters: []models.ParameterDefinition{
			{Name: "count", Type: models.ParamNumber, Required: true},
		},
	}
	params := map[string]interface{}{
		"count": "not-a-number",
	}

	v := NewParameterValidator()
	result := v.Validate(schema, params)

	if result.Valid {
		t.Fatal("expected invalid result")
	}
	if result.Errors[0].Code != "invalid_type" {
		t.Errorf("expected code 'invalid_type', got %q", result.Errors[0].Code)
	}
}

func TestValidate_WrongType_Boolean_Fails(t *testing.T) {
	schema := models.ParameterSchema{
		Parameters: []models.ParameterDefinition{
			{Name: "verbose", Type: models.ParamBoolean, Required: true},
		},
	}
	params := map[string]interface{}{
		"verbose": "yes",
	}

	v := NewParameterValidator()
	result := v.Validate(schema, params)

	if result.Valid {
		t.Fatal("expected invalid result")
	}
	if result.Errors[0].Code != "invalid_type" {
		t.Errorf("expected code 'invalid_type', got %q", result.Errors[0].Code)
	}
}

func TestValidate_WrongType_Enum_Fails(t *testing.T) {
	schema := models.ParameterSchema{
		Parameters: []models.ParameterDefinition{
			{
				Name:     "env",
				Type:     models.ParamEnum,
				Required: true,
				Validation: &models.ValidationRules{
					EnumValues: []string{"dev", "staging", "prod"},
				},
			},
		},
	}
	params := map[string]interface{}{
		"env": float64(1),
	}

	v := NewParameterValidator()
	result := v.Validate(schema, params)

	if result.Valid {
		t.Fatal("expected invalid result")
	}
	if result.Errors[0].Code != "invalid_type" {
		t.Errorf("expected code 'invalid_type', got %q", result.Errors[0].Code)
	}
}

func TestValidate_PatternConstraint_Passes(t *testing.T) {
	schema := models.ParameterSchema{
		Parameters: []models.ParameterDefinition{
			{
				Name:     "db_name",
				Type:     models.ParamString,
				Required: true,
				Validation: &models.ValidationRules{
					Pattern: `^[a-z_]+$`,
				},
			},
		},
	}
	params := map[string]interface{}{
		"db_name": "my_database",
	}

	v := NewParameterValidator()
	result := v.Validate(schema, params)

	if !result.Valid {
		t.Errorf("expected valid, got errors: %v", result.Errors)
	}
}

func TestValidate_PatternConstraint_Fails(t *testing.T) {
	schema := models.ParameterSchema{
		Parameters: []models.ParameterDefinition{
			{
				Name:     "db_name",
				Type:     models.ParamString,
				Required: true,
				Validation: &models.ValidationRules{
					Pattern: `^[a-z_]+$`,
				},
			},
		},
	}
	params := map[string]interface{}{
		"db_name": "INVALID-NAME!",
	}

	v := NewParameterValidator()
	result := v.Validate(schema, params)

	if result.Valid {
		t.Fatal("expected invalid result")
	}
	if result.Errors[0].Code != "validation_failed" {
		t.Errorf("expected code 'validation_failed', got %q", result.Errors[0].Code)
	}
}

func TestValidate_MinConstraint_Fails(t *testing.T) {
	schema := models.ParameterSchema{
		Parameters: []models.ParameterDefinition{
			{
				Name:     "count",
				Type:     models.ParamNumber,
				Required: true,
				Validation: &models.ValidationRules{
					Min: float64Ptr(1),
				},
			},
		},
	}
	params := map[string]interface{}{
		"count": float64(0),
	}

	v := NewParameterValidator()
	result := v.Validate(schema, params)

	if result.Valid {
		t.Fatal("expected invalid result")
	}
	if result.Errors[0].Code != "validation_failed" {
		t.Errorf("expected code 'validation_failed', got %q", result.Errors[0].Code)
	}
}

func TestValidate_MaxConstraint_Fails(t *testing.T) {
	schema := models.ParameterSchema{
		Parameters: []models.ParameterDefinition{
			{
				Name:     "count",
				Type:     models.ParamNumber,
				Required: true,
				Validation: &models.ValidationRules{
					Max: float64Ptr(100),
				},
			},
		},
	}
	params := map[string]interface{}{
		"count": float64(101),
	}

	v := NewParameterValidator()
	result := v.Validate(schema, params)

	if result.Valid {
		t.Fatal("expected invalid result")
	}
	if result.Errors[0].Code != "validation_failed" {
		t.Errorf("expected code 'validation_failed', got %q", result.Errors[0].Code)
	}
}

func TestValidate_MinMaxConstraint_Passes(t *testing.T) {
	schema := models.ParameterSchema{
		Parameters: []models.ParameterDefinition{
			{
				Name:     "count",
				Type:     models.ParamNumber,
				Required: true,
				Validation: &models.ValidationRules{
					Min: float64Ptr(1),
					Max: float64Ptr(100),
				},
			},
		},
	}
	params := map[string]interface{}{
		"count": float64(50),
	}

	v := NewParameterValidator()
	result := v.Validate(schema, params)

	if !result.Valid {
		t.Errorf("expected valid, got errors: %v", result.Errors)
	}
}

func TestValidate_EnumConstraint_Passes(t *testing.T) {
	schema := models.ParameterSchema{
		Parameters: []models.ParameterDefinition{
			{
				Name:     "env",
				Type:     models.ParamEnum,
				Required: true,
				Validation: &models.ValidationRules{
					EnumValues: []string{"dev", "staging", "prod"},
				},
			},
		},
	}
	params := map[string]interface{}{
		"env": "staging",
	}

	v := NewParameterValidator()
	result := v.Validate(schema, params)

	if !result.Valid {
		t.Errorf("expected valid, got errors: %v", result.Errors)
	}
}

func TestValidate_EnumConstraint_InvalidValue_Fails(t *testing.T) {
	schema := models.ParameterSchema{
		Parameters: []models.ParameterDefinition{
			{
				Name:     "env",
				Type:     models.ParamEnum,
				Required: true,
				Validation: &models.ValidationRules{
					EnumValues: []string{"dev", "staging", "prod"},
				},
			},
		},
	}
	params := map[string]interface{}{
		"env": "unknown",
	}

	v := NewParameterValidator()
	result := v.Validate(schema, params)

	if result.Valid {
		t.Fatal("expected invalid result")
	}
	if result.Errors[0].Code != "validation_failed" {
		t.Errorf("expected code 'validation_failed', got %q", result.Errors[0].Code)
	}
}

func TestValidate_UnknownParameter_Fails(t *testing.T) {
	schema := models.ParameterSchema{
		Parameters: []models.ParameterDefinition{
			{Name: "name", Type: models.ParamString, Required: true},
		},
	}
	params := map[string]interface{}{
		"name":    "test",
		"unknown": "value",
	}

	v := NewParameterValidator()
	result := v.Validate(schema, params)

	if result.Valid {
		t.Fatal("expected invalid result")
	}

	foundUnknown := false
	for _, err := range result.Errors {
		if err.Code == "unknown_parameter" && err.ParameterName == "unknown" {
			foundUnknown = true
			break
		}
	}
	if !foundUnknown {
		t.Errorf("expected unknown_parameter error for 'unknown', got: %v", result.Errors)
	}
}

func TestValidate_MaxLength_Passes(t *testing.T) {
	schema := models.ParameterSchema{
		Parameters: []models.ParameterDefinition{
			{
				Name:     "name",
				Type:     models.ParamString,
				Required: true,
				Validation: &models.ValidationRules{
					MaxLength: intPtr(10),
				},
			},
		},
	}
	params := map[string]interface{}{
		"name": "short",
	}

	v := NewParameterValidator()
	result := v.Validate(schema, params)

	if !result.Valid {
		t.Errorf("expected valid, got errors: %v", result.Errors)
	}
}

func TestValidate_MaxLength_Fails(t *testing.T) {
	schema := models.ParameterSchema{
		Parameters: []models.ParameterDefinition{
			{
				Name:     "name",
				Type:     models.ParamString,
				Required: true,
				Validation: &models.ValidationRules{
					MaxLength: intPtr(5),
				},
			},
		},
	}
	params := map[string]interface{}{
		"name": "toolongvalue",
	}

	v := NewParameterValidator()
	result := v.Validate(schema, params)

	if result.Valid {
		t.Fatal("expected invalid result")
	}
	if result.Errors[0].Code != "validation_failed" {
		t.Errorf("expected code 'validation_failed', got %q", result.Errors[0].Code)
	}
}

func TestValidate_DefaultValueApplied_WhenNotProvided(t *testing.T) {
	schema := models.ParameterSchema{
		Parameters: []models.ParameterDefinition{
			{Name: "name", Type: models.ParamString, Required: false, DefaultValue: "default_name"},
			{Name: "count", Type: models.ParamNumber, Required: true},
		},
	}
	params := map[string]interface{}{
		"count": float64(10),
	}

	v := NewParameterValidator()
	result := v.Validate(schema, params)

	if !result.Valid {
		t.Errorf("expected valid (optional param with default not provided), got errors: %v", result.Errors)
	}
}

func TestValidate_IntegerAsNumber_Passes(t *testing.T) {
	schema := models.ParameterSchema{
		Parameters: []models.ParameterDefinition{
			{Name: "count", Type: models.ParamNumber, Required: true},
		},
	}
	params := map[string]interface{}{
		"count": 42, // int, not float64
	}

	v := NewParameterValidator()
	result := v.Validate(schema, params)

	if !result.Valid {
		t.Errorf("expected valid (int should be accepted as number), got errors: %v", result.Errors)
	}
}

func TestValidate_NilValue_RequiredParam_Fails(t *testing.T) {
	schema := models.ParameterSchema{
		Parameters: []models.ParameterDefinition{
			{Name: "name", Type: models.ParamString, Required: true},
		},
	}
	params := map[string]interface{}{
		"name": nil,
	}

	v := NewParameterValidator()
	result := v.Validate(schema, params)

	if result.Valid {
		t.Fatal("expected invalid result for nil required param")
	}
	if result.Errors[0].Code != "required" {
		t.Errorf("expected code 'required', got %q", result.Errors[0].Code)
	}
}

func TestValidate_EmptySchema_EmptyParams_Passes(t *testing.T) {
	schema := models.ParameterSchema{
		Parameters: []models.ParameterDefinition{},
	}
	params := map[string]interface{}{}

	v := NewParameterValidator()
	result := v.Validate(schema, params)

	if !result.Valid {
		t.Errorf("expected valid for empty schema and params, got errors: %v", result.Errors)
	}
}

func TestValidate_MultipleErrors(t *testing.T) {
	schema := models.ParameterSchema{
		Parameters: []models.ParameterDefinition{
			{Name: "name", Type: models.ParamString, Required: true},
			{Name: "count", Type: models.ParamNumber, Required: true},
		},
	}
	params := map[string]interface{}{
		"extra": "unknown",
	}

	v := NewParameterValidator()
	result := v.Validate(schema, params)

	if result.Valid {
		t.Fatal("expected invalid result")
	}
	// Should have: unknown_parameter for "extra", required for "name", required for "count"
	if len(result.Errors) != 3 {
		t.Errorf("expected 3 errors, got %d: %v", len(result.Errors), result.Errors)
	}
}

// --- BuildCommandArgs Tests ---

func TestBuildCommandArgs_SimpleSubstitution(t *testing.T) {
	schema := models.ParameterSchema{
		Parameters: []models.ParameterDefinition{
			{Name: "db_name", Type: models.ParamString},
		},
	}
	params := map[string]interface{}{
		"db_name": "production",
	}

	v := NewParameterValidator()
	args, err := v.BuildCommandArgs("pg_dump {{db_name}}", schema, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{"pg_dump", "production"}
	if len(args) != len(expected) {
		t.Fatalf("expected %d args, got %d: %v", len(expected), len(args), args)
	}
	for i, arg := range args {
		if arg != expected[i] {
			t.Errorf("arg[%d]: expected %q, got %q", i, expected[i], arg)
		}
	}
}

func TestBuildCommandArgs_MultipleParameters(t *testing.T) {
	schema := models.ParameterSchema{
		Parameters: []models.ParameterDefinition{
			{Name: "host", Type: models.ParamString},
			{Name: "port", Type: models.ParamNumber},
			{Name: "db", Type: models.ParamString},
		},
	}
	params := map[string]interface{}{
		"host": "localhost",
		"port": float64(5432),
		"db":   "mydb",
	}

	v := NewParameterValidator()
	args, err := v.BuildCommandArgs("psql -h {{host}} -p {{port}} -d {{db}}", schema, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{"psql", "-h", "localhost", "-p", "5432", "-d", "mydb"}
	if len(args) != len(expected) {
		t.Fatalf("expected %d args, got %d: %v", len(expected), len(args), args)
	}
	for i, arg := range args {
		if arg != expected[i] {
			t.Errorf("arg[%d]: expected %q, got %q", i, expected[i], arg)
		}
	}
}

func TestBuildCommandArgs_ShellMetacharacters_IsolatedElement(t *testing.T) {
	schema := models.ParameterSchema{
		Parameters: []models.ParameterDefinition{
			{Name: "input", Type: models.ParamString},
		},
	}

	// Shell injection attempts — each should become a single isolated argument.
	injectionAttempts := []string{
		"; rm -rf /",
		"| cat /etc/passwd",
		"$(whoami)",
		"`whoami`",
		"&& echo pwned",
		"|| echo pwned",
		"> /tmp/evil",
		"< /etc/shadow",
		"\necho injected",
		"hello; world",
	}

	v := NewParameterValidator()
	for _, attempt := range injectionAttempts {
		params := map[string]interface{}{
			"input": attempt,
		}
		args, err := v.BuildCommandArgs("echo {{input}}", schema, params)
		if err != nil {
			t.Fatalf("unexpected error for input %q: %v", attempt, err)
		}

		// The result should be exactly ["echo", "<the_attempt_string>"]
		// The injection string is a single isolated element, not split or interpreted.
		if len(args) != 2 {
			t.Errorf("for input %q: expected 2 args, got %d: %v", attempt, len(args), args)
			continue
		}
		if args[0] != "echo" {
			t.Errorf("for input %q: expected args[0]='echo', got %q", attempt, args[0])
		}
		if args[1] != attempt {
			t.Errorf("for input %q: expected args[1]=%q, got %q", attempt, attempt, args[1])
		}
	}
}

func TestBuildCommandArgs_NoPlaceholders(t *testing.T) {
	schema := models.ParameterSchema{}
	params := map[string]interface{}{}

	v := NewParameterValidator()
	args, err := v.BuildCommandArgs("ls -la /tmp", schema, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{"ls", "-la", "/tmp"}
	if len(args) != len(expected) {
		t.Fatalf("expected %d args, got %d: %v", len(expected), len(args), args)
	}
	for i, arg := range args {
		if arg != expected[i] {
			t.Errorf("arg[%d]: expected %q, got %q", i, expected[i], arg)
		}
	}
}

func TestBuildCommandArgs_BooleanParam(t *testing.T) {
	schema := models.ParameterSchema{
		Parameters: []models.ParameterDefinition{
			{Name: "verbose", Type: models.ParamBoolean},
		},
	}
	params := map[string]interface{}{
		"verbose": true,
	}

	v := NewParameterValidator()
	args, err := v.BuildCommandArgs("cmd --verbose={{verbose}}", schema, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{"cmd", "--verbose=true"}
	if len(args) != len(expected) {
		t.Fatalf("expected %d args, got %d: %v", len(expected), len(args), args)
	}
	for i, arg := range args {
		if arg != expected[i] {
			t.Errorf("arg[%d]: expected %q, got %q", i, expected[i], arg)
		}
	}
}

func TestBuildCommandArgs_DefaultValue_UsedWhenNotProvided(t *testing.T) {
	schema := models.ParameterSchema{
		Parameters: []models.ParameterDefinition{
			{Name: "format", Type: models.ParamString, DefaultValue: "json"},
		},
	}
	params := map[string]interface{}{} // No value provided

	v := NewParameterValidator()
	args, err := v.BuildCommandArgs("export --format {{format}}", schema, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{"export", "--format", "json"}
	if len(args) != len(expected) {
		t.Fatalf("expected %d args, got %d: %v", len(expected), len(args), args)
	}
	for i, arg := range args {
		if arg != expected[i] {
			t.Errorf("arg[%d]: expected %q, got %q", i, expected[i], arg)
		}
	}
}

func TestBuildCommandArgs_EmptyCommandString(t *testing.T) {
	schema := models.ParameterSchema{}
	params := map[string]interface{}{}

	v := NewParameterValidator()
	args, err := v.BuildCommandArgs("", schema, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(args) != 0 {
		t.Errorf("expected 0 args for empty command, got %d: %v", len(args), args)
	}
}
