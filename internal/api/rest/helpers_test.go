package rest

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// assertJSONError asserts that the response body contains a JSON object with an "error" key.
func assertJSONError(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("response body is not valid JSON: %v (body=%q)", err, w.Body.String())
	}
	if _, ok := body["error"]; !ok {
		t.Errorf("expected 'error' key in response body, got: %v", body)
	}
}

// withBody returns a copy of req with its body replaced by the given string and
// Content-Type set to application/json.
func withBody(req *http.Request, body string) *http.Request {
	req2 := req.Clone(req.Context())
	req2.Body = io.NopCloser(strings.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	return req2
}

// ── parseScopeString ──────────────────────────────────────────────────────────

func TestParseScopeString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		input      string
		wantKind   string
		wantExtID  string
		wantErrMsg string
	}{
		{
			name:      "valid scope",
			input:     "team:eng-platform",
			wantKind:  "team",
			wantExtID: "eng-platform",
		},
		{
			name:      "value contains colon",
			input:     "repo:github.com/org/project",
			wantKind:  "repo",
			wantExtID: "github.com/org/project",
		},
		{
			name:      "minimal — single char on each side",
			input:     "a:b",
			wantKind:  "a",
			wantExtID: "b",
		},
		{
			name:      "empty external ID",
			input:     "team:",
			wantKind:  "team",
			wantExtID: "",
		},
		{
			name:       "missing colon",
			input:      "nocolon",
			wantErrMsg: "missing ':' separator",
		},
		{
			name:       "empty string",
			input:      "",
			wantErrMsg: "empty scope string",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			kind, extID, err := parseScopeString(tt.input)
			if tt.wantErrMsg != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErrMsg)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if kind != tt.wantKind {
				t.Errorf("kind = %q, want %q", kind, tt.wantKind)
			}
			if extID != tt.wantExtID {
				t.Errorf("externalID = %q, want %q", extID, tt.wantExtID)
			}
		})
	}
}

// ── paginationFromRequest ─────────────────────────────────────────────────────

func TestPaginationFromRequest(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		query      string
		wantLimit  int
		wantOffset int
		wantCursor string
	}{
		{
			name:       "no params — defaults applied",
			query:      "",
			wantLimit:  20,
			wantOffset: 0,
		},
		{
			name:       "valid limit and offset",
			query:      "limit=10&offset=5",
			wantLimit:  10,
			wantOffset: 5,
		},
		{
			name:       "limit of 1 — minimum valid",
			query:      "limit=1",
			wantLimit:  1,
			wantOffset: 0,
		},
		{
			name:       "limit of 100 — maximum valid",
			query:      "limit=100",
			wantLimit:  100,
			wantOffset: 0,
		},
		{
			name:       "limit 0 — clamped to default",
			query:      "limit=0",
			wantLimit:  20,
			wantOffset: 0,
		},
		{
			name:       "limit 101 — clamped to default",
			query:      "limit=101",
			wantLimit:  20,
			wantOffset: 0,
		},
		{
			name:       "negative limit — clamped to default",
			query:      "limit=-1",
			wantLimit:  20,
			wantOffset: 0,
		},
		{
			name:       "negative offset — clamped to 0",
			query:      "offset=-5",
			wantLimit:  20,
			wantOffset: 0,
		},
		{
			name:       "non-numeric limit — default applied",
			query:      "limit=abc",
			wantLimit:  20,
			wantOffset: 0,
		},
		{
			name:       "cursor forwarded",
			query:      "cursor=tok_abc123",
			wantLimit:  20,
			wantOffset: 0,
			wantCursor: "tok_abc123",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, "/?"+tt.query, nil)
			p := paginationFromRequest(req)
			if p.Limit != tt.wantLimit {
				t.Errorf("Limit = %d, want %d", p.Limit, tt.wantLimit)
			}
			if p.Offset != tt.wantOffset {
				t.Errorf("Offset = %d, want %d", p.Offset, tt.wantOffset)
			}
			if p.Cursor != tt.wantCursor {
				t.Errorf("Cursor = %q, want %q", p.Cursor, tt.wantCursor)
			}
		})
	}
}

// ── uuidParam ─────────────────────────────────────────────────────────────────

// requestWithChiParam builds an *http.Request with a chi URL parameter injected.
func requestWithChiParam(t *testing.T, paramName, paramValue string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(paramName, paramValue)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func TestUUIDParam(t *testing.T) {
	t.Parallel()
	want := uuid.New()

	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{
			name:  "valid UUID",
			value: want.String(),
		},
		{
			name:    "invalid string",
			value:   "not-a-uuid",
			wantErr: true,
		},
		{
			name:    "empty string (missing param)",
			value:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := requestWithChiParam(t, "id", tt.value)

			got, err := uuidParam(req, "id")
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error for value %q, got nil (uuid=%v)", tt.value, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != want {
				t.Errorf("got %v, want %v", got, want)
			}
		})
	}
}

// ── entityRequestsToInput ─────────────────────────────────────────────────────

func TestEntityRequestsToInput(t *testing.T) {
	t.Parallel()

	t.Run("empty slice", func(t *testing.T) {
		t.Parallel()
		got := entityRequestsToInput([]entityRequest{})
		if len(got) != 0 {
			t.Errorf("len = %d, want 0", len(got))
		}
	})

	t.Run("nil slice", func(t *testing.T) {
		t.Parallel()
		got := entityRequestsToInput(nil)
		if len(got) != 0 {
			t.Errorf("len = %d, want 0", len(got))
		}
	})

	t.Run("single entry with both fields", func(t *testing.T) {
		t.Parallel()
		got := entityRequestsToInput([]entityRequest{
			{Name: "postgresql", Type: "technology"},
		})
		if len(got) != 1 {
			t.Fatalf("len = %d, want 1", len(got))
		}
		if got[0].Name != "postgresql" {
			t.Errorf("Name = %q, want %q", got[0].Name, "postgresql")
		}
		if got[0].Type != "technology" {
			t.Errorf("Type = %q, want %q", got[0].Type, "technology")
		}
	})

	t.Run("entry with empty name is skipped", func(t *testing.T) {
		t.Parallel()
		got := entityRequestsToInput([]entityRequest{
			{Name: "", Type: "concept"},
			{Name: "redis", Type: "technology"},
		})
		if len(got) != 1 {
			t.Fatalf("len = %d, want 1 (empty-name entry must be skipped)", len(got))
		}
		if got[0].Name != "redis" {
			t.Errorf("Name = %q, want %q", got[0].Name, "redis")
		}
	})

	t.Run("multiple entries all valid", func(t *testing.T) {
		t.Parallel()
		reqs := []entityRequest{
			{Name: "alice", Type: "person"},
			{Name: "payments-service", Type: "service"},
			{Name: "src/auth.go", Type: "file"},
		}
		got := entityRequestsToInput(reqs)
		if len(got) != len(reqs) {
			t.Fatalf("len = %d, want %d", len(got), len(reqs))
		}
		for i, r := range reqs {
			if got[i].Name != r.Name || got[i].Type != r.Type {
				t.Errorf("[%d] got {%q,%q}, want {%q,%q}", i, got[i].Name, got[i].Type, r.Name, r.Type)
			}
		}
	})

	t.Run("entry with type but empty name is skipped", func(t *testing.T) {
		t.Parallel()
		got := entityRequestsToInput([]entityRequest{{Name: "", Type: "technology"}})
		if len(got) != 0 {
			t.Errorf("len = %d, want 0 (empty-name must be skipped regardless of type)", len(got))
		}
	})
}
