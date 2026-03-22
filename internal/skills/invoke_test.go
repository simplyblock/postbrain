package skills

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/simplyblock/postbrain/internal/db"
)

func buildSkill(body string, params []db.SkillParameter) *db.Skill {
	raw, _ := json.Marshal(params)
	return &db.Skill{Body: body, Parameters: raw}
}

func TestInvoke_AllParamsValid(t *testing.T) {
	t.Parallel()
	skill := buildSkill("Hello $NAME, you are {{age}} years old.", []db.SkillParameter{
		{Name: "name", Type: "string", Required: true, Description: "Name"},
		{Name: "age", Type: "integer", Required: true, Description: "Age"},
	})
	result, err := Invoke(skill, map[string]any{
		"name": "Alice",
		"age":  float64(30),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Alice") {
		t.Error("expected NAME substitution")
	}
	if !strings.Contains(result, "30") {
		t.Error("expected age substitution")
	}
}

func TestInvoke_DollarSubstitution(t *testing.T) {
	t.Parallel()
	skill := buildSkill("Run $CMD on the project.", []db.SkillParameter{
		{Name: "cmd", Type: "string", Required: true, Description: "Command"},
	})
	result, err := Invoke(skill, map[string]any{"cmd": "build"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Run build on the project." {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestInvoke_DoubleBraceSubstitution(t *testing.T) {
	t.Parallel()
	skill := buildSkill("Deploy {{env}} environment.", []db.SkillParameter{
		{Name: "env", Type: "string", Required: true, Description: "Environment"},
	})
	result, err := Invoke(skill, map[string]any{"env": "production"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Deploy production environment." {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestInvoke_MissingRequired(t *testing.T) {
	t.Parallel()
	skill := buildSkill("Do $THING.", []db.SkillParameter{
		{Name: "thing", Type: "string", Required: true, Description: "The thing"},
	})
	_, err := Invoke(skill, map[string]any{})
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
	if len(ve.Fields) != 1 || ve.Fields[0].Name != "thing" {
		t.Errorf("unexpected fields: %+v", ve.Fields)
	}
}

func TestInvoke_WrongTypeInteger(t *testing.T) {
	t.Parallel()
	skill := buildSkill("Count $N.", []db.SkillParameter{
		{Name: "n", Type: "integer", Required: true, Description: "Count"},
	})
	_, err := Invoke(skill, map[string]any{"n": "not-a-number"})
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
	if len(ve.Fields) != 1 || ve.Fields[0].Name != "n" {
		t.Errorf("unexpected fields: %+v", ve.Fields)
	}
}

func TestInvoke_EnumValueNotInList(t *testing.T) {
	t.Parallel()
	skill := buildSkill("Focus on $FOCUS.", []db.SkillParameter{
		{Name: "focus", Type: "enum", Required: true, Description: "Focus area", Values: []string{"security", "style"}},
	})
	_, err := Invoke(skill, map[string]any{"focus": "performance"})
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
	if len(ve.Fields) != 1 || ve.Fields[0].Name != "focus" {
		t.Errorf("unexpected fields: %+v", ve.Fields)
	}
}

func TestInvoke_MultipleErrors(t *testing.T) {
	t.Parallel()
	skill := buildSkill("$A $B", []db.SkillParameter{
		{Name: "a", Type: "string", Required: true, Description: "A"},
		{Name: "b", Type: "integer", Required: true, Description: "B"},
	})
	// b is missing, and a is the wrong type (integer expected, string given for b... actually a is fine as string)
	// Let's make both fail: a is missing, b is wrong type
	_, err := Invoke(skill, map[string]any{"b": "oops"})
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
	if len(ve.Fields) < 2 {
		t.Errorf("expected at least 2 field errors, got %d: %+v", len(ve.Fields), ve.Fields)
	}
}
