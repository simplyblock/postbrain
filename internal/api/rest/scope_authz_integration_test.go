//go:build integration

package rest_test

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/api/rest"
	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/config"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

func TestREST_ScopeAuthz_WriteEndpoints(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	cfg := &config.Config{}

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "rest-scope-authz-"+uuid.New().String())
	allowed := testhelper.CreateTestScope(t, pool, "project", "rest-scope-allowed-"+uuid.New().String(), nil, principal.ID)
	blocked := testhelper.CreateTestScope(t, pool, "project", "rest-scope-blocked-"+uuid.New().String(), nil, principal.ID)
	testhelper.CreateTestEmbeddingModel(t, pool)

	rawToken, hashToken, err := auth.GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.CreateToken(ctx, pool, principal.ID, hashToken, "rest-scope-authz-token", []uuid.UUID{allowed.ID}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	handler := rest.NewRouter(pool, svc, cfg).Handler()
	srv := httptest.NewServer(handler)
	defer srv.Close()

	authHeader := "Bearer " + rawToken

	type endpointCase struct {
		name       string
		path       string
		reqBuilder func(scopeStr string) (*http.Request, error)
	}

	cases := []endpointCase{
		{
			name: "create memory",
			path: "/v1/memories",
			reqBuilder: func(scopeStr string) (*http.Request, error) {
				body := map[string]any{
					"content":     "scope authz test memory",
					"scope":       scopeStr,
					"memory_type": "semantic",
					"importance":  0.5,
				}
				b, _ := json.Marshal(body)
				req, err := http.NewRequest(http.MethodPost, srv.URL+"/v1/memories", bytes.NewReader(b))
				if err != nil {
					return nil, err
				}
				req.Header.Set("Content-Type", "application/json")
				return req, nil
			},
		},
		{
			name: "create artifact",
			path: "/v1/knowledge",
			reqBuilder: func(scopeStr string) (*http.Request, error) {
				body := map[string]any{
					"title":          "scope authz artifact",
					"content":        "artifact content",
					"knowledge_type": "semantic",
					"scope":          scopeStr,
				}
				b, _ := json.Marshal(body)
				req, err := http.NewRequest(http.MethodPost, srv.URL+"/v1/knowledge", bytes.NewReader(b))
				if err != nil {
					return nil, err
				}
				req.Header.Set("Content-Type", "application/json")
				return req, nil
			},
		},
		{
			name: "create skill",
			path: "/v1/skills",
			reqBuilder: func(scopeStr string) (*http.Request, error) {
				body := map[string]any{
					"scope": scopeStr,
					"slug":  "skill-" + uuid.New().String(),
					"name":  "Scope Authz Skill",
				}
				b, _ := json.Marshal(body)
				req, err := http.NewRequest(http.MethodPost, srv.URL+"/v1/skills", bytes.NewReader(b))
				if err != nil {
					return nil, err
				}
				req.Header.Set("Content-Type", "application/json")
				return req, nil
			},
		},
		{
			name: "create collection",
			path: "/v1/collections",
			reqBuilder: func(scopeStr string) (*http.Request, error) {
				body := map[string]any{
					"scope": scopeStr,
					"slug":  "collection-" + uuid.New().String(),
					"name":  "Scope Authz Collection",
				}
				b, _ := json.Marshal(body)
				req, err := http.NewRequest(http.MethodPost, srv.URL+"/v1/collections", bytes.NewReader(b))
				if err != nil {
					return nil, err
				}
				req.Header.Set("Content-Type", "application/json")
				return req, nil
			},
		},
		{
			name: "upload knowledge",
			path: "/v1/knowledge/upload",
			reqBuilder: func(scopeStr string) (*http.Request, error) {
				var body bytes.Buffer
				w := multipart.NewWriter(&body)
				fw, err := w.CreateFormFile("file", "scope-authz.txt")
				if err != nil {
					return nil, err
				}
				if _, err := fw.Write([]byte("scope authz upload content")); err != nil {
					return nil, err
				}
				if err := w.WriteField("scope", scopeStr); err != nil {
					return nil, err
				}
				if err := w.Close(); err != nil {
					return nil, err
				}
				req, err := http.NewRequest(http.MethodPost, srv.URL+"/v1/knowledge/upload", &body)
				if err != nil {
					return nil, err
				}
				req.Header.Set("Content-Type", w.FormDataContentType())
				return req, nil
			},
		},
		{
			name: "create session",
			path: "/v1/sessions",
			reqBuilder: func(scopeStr string) (*http.Request, error) {
				body := map[string]any{"scope": scopeStr}
				b, _ := json.Marshal(body)
				req, err := http.NewRequest(http.MethodPost, srv.URL+"/v1/sessions", bytes.NewReader(b))
				if err != nil {
					return nil, err
				}
				req.Header.Set("Content-Type", "application/json")
				return req, nil
			},
		},
	}

	scopeAllowed := "project:" + allowed.ExternalID
	scopeBlocked := "project:" + blocked.ExternalID

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name+" authorized scope returns success", func(t *testing.T) {
			req, err := tc.reqBuilder(scopeAllowed)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", authHeader)

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusCreated {
				t.Fatalf("%s status = %d, want %d", tc.path, resp.StatusCode, http.StatusCreated)
			}
		})

		t.Run(tc.name+" unauthorized scope returns forbidden", func(t *testing.T) {
			req, err := tc.reqBuilder(scopeBlocked)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", authHeader)

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusForbidden {
				t.Fatalf("%s status = %d, want %d", tc.path, resp.StatusCode, http.StatusForbidden)
			}
			var out map[string]any
			_ = json.NewDecoder(resp.Body).Decode(&out)
			errMsg, _ := out["error"].(string)
			if !strings.Contains(errMsg, "scope access denied") {
				t.Fatalf("%s error = %q, want scope access denied", tc.path, errMsg)
			}
		})

		t.Run(tc.name+" malformed scope returns bad request", func(t *testing.T) {
			req, err := tc.reqBuilder("invalid-scope-format")
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", authHeader)

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("%s status = %d, want %d", tc.path, resp.StatusCode, http.StatusBadRequest)
			}
		})
	}
}
