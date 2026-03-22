package skills

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/simplyblock/postbrain/internal/db"
)

// ValidationError is returned when one or more parameter constraints are violated.
type ValidationError struct {
	Fields []FieldError
}

// FieldError describes a single parameter validation failure.
type FieldError struct {
	Name   string
	Reason string
}

func (e *ValidationError) Error() string {
	parts := make([]string, len(e.Fields))
	for i, f := range e.Fields {
		parts[i] = fmt.Sprintf("%s: %s", f.Name, f.Reason)
	}
	return "skills: validation errors: " + strings.Join(parts, "; ")
}

// Invoke validates params against the skill's parameter schema, substitutes them
// into the body, and returns the expanded body.
func Invoke(skill *db.Skill, params map[string]any) (string, error) {
	var schema []db.SkillParameter
	if len(skill.Parameters) > 0 {
		if err := json.Unmarshal(skill.Parameters, &schema); err != nil {
			return "", fmt.Errorf("skills: unmarshal parameters: %w", err)
		}
	}

	var fieldErrors []FieldError

	// Validate all parameters in schema.
	for _, p := range schema {
		val, present := params[p.Name]
		if !present {
			if p.Required {
				fieldErrors = append(fieldErrors, FieldError{Name: p.Name, Reason: "required parameter is missing"})
			}
			continue
		}

		switch p.Type {
		case "string":
			if _, ok := val.(string); !ok {
				fieldErrors = append(fieldErrors, FieldError{Name: p.Name, Reason: "expected string"})
			}
		case "integer":
			switch val.(type) {
			case int, int64, float64:
				// valid
			default:
				fieldErrors = append(fieldErrors, FieldError{Name: p.Name, Reason: "expected integer"})
			}
		case "boolean":
			if _, ok := val.(bool); !ok {
				fieldErrors = append(fieldErrors, FieldError{Name: p.Name, Reason: "expected boolean"})
			}
		case "enum":
			s, ok := val.(string)
			if !ok {
				fieldErrors = append(fieldErrors, FieldError{Name: p.Name, Reason: "expected string for enum"})
				continue
			}
			if !containsString(p.Values, s) {
				fieldErrors = append(fieldErrors, FieldError{
					Name:   p.Name,
					Reason: fmt.Sprintf("value %q is not in allowed values %v", s, p.Values),
				})
			}
		}
	}

	if len(fieldErrors) > 0 {
		return "", &ValidationError{Fields: fieldErrors}
	}

	// Substitute parameters into body.
	body := skill.Body
	for _, p := range schema {
		val, present := params[p.Name]
		if !present {
			continue
		}
		strVal := anyToString(val)
		upper := strings.ToUpper(p.Name)
		lower := strings.ToLower(p.Name)
		body = strings.ReplaceAll(body, "$"+upper, strVal)
		body = strings.ReplaceAll(body, "{{"+lower+"}}", strVal)
	}

	return body, nil
}

// anyToString converts a parameter value to its string representation.
func anyToString(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case bool:
		if val {
			return "true"
		}
		return "false"
	case int:
		return strconv.FormatInt(int64(val), 10)
	case int64:
		return strconv.FormatInt(val, 10)
	case float64:
		// JSON numbers unmarshal as float64; treat as integer if it is whole.
		if val == float64(int64(val)) {
			return strconv.FormatInt(int64(val), 10)
		}
		return strconv.FormatFloat(val, 'f', -1, 64)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// containsString reports whether s is in the slice.
func containsString(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
