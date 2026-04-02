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
	"time"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/api/rest"
	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/config"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/memory"
	"github.com/simplyblock/postbrain/internal/principals"
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

func TestREST_ScopeAuthz_IDBasedEndpoints(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	cfg := &config.Config{}

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "rest-scope-id-authz-"+uuid.New().String())
	allowed := testhelper.CreateTestScope(t, pool, "project", "rest-scope-id-allowed-"+uuid.New().String(), nil, principal.ID)
	blocked := testhelper.CreateTestScope(t, pool, "project", "rest-scope-id-blocked-"+uuid.New().String(), nil, principal.ID)
	testhelper.CreateTestEmbeddingModel(t, pool)

	allowedMemory := testhelper.CreateTestMemory(t, pool, allowed.ID, principal.ID, "allowed memory")
	blockedMemory := testhelper.CreateTestMemory(t, pool, blocked.ID, principal.ID, "blocked memory")

	allowedArtifact := testhelper.CreateTestArtifact(t, pool, allowed.ID, principal.ID, "allowed artifact")
	blockedArtifact := testhelper.CreateTestArtifact(t, pool, blocked.ID, principal.ID, "blocked artifact")
	allowedDraftArtifact, err := db.CreateArtifact(ctx, pool, &db.KnowledgeArtifact{
		KnowledgeType: "semantic",
		OwnerScopeID:  allowed.ID,
		AuthorID:      principal.ID,
		Visibility:    "team",
		Status:        "draft",
		Title:         "allowed draft artifact",
		Content:       "allowed draft content",
	})
	if err != nil {
		t.Fatal(err)
	}
	blockedDraftArtifact, err := db.CreateArtifact(ctx, pool, &db.KnowledgeArtifact{
		KnowledgeType: "semantic",
		OwnerScopeID:  blocked.ID,
		AuthorID:      principal.ID,
		Visibility:    "team",
		Status:        "draft",
		Title:         "blocked draft artifact",
		Content:       "blocked draft content",
	})
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	skillParams, err := json.Marshal([]db.SkillParameter{})
	if err != nil {
		t.Fatal(err)
	}
	allowedSkill, err := db.CreateSkill(ctx, pool, &db.Skill{
		ScopeID:        allowed.ID,
		AuthorID:       principal.ID,
		Slug:           "allowed-skill-" + uuid.New().String(),
		Name:           "Allowed Skill",
		Description:    "Allowed skill",
		AgentTypes:     []string{"any"},
		Body:           "Allowed skill body",
		Parameters:     skillParams,
		Visibility:     "team",
		Status:         "published",
		PublishedAt:    &now,
		ReviewRequired: 1,
		Version:        1,
	})
	if err != nil {
		t.Fatal(err)
	}
	blockedSkill, err := db.CreateSkill(ctx, pool, &db.Skill{
		ScopeID:        blocked.ID,
		AuthorID:       principal.ID,
		Slug:           "blocked-skill-" + uuid.New().String(),
		Name:           "Blocked Skill",
		Description:    "Blocked skill",
		AgentTypes:     []string{"any"},
		Body:           "Blocked skill body",
		Parameters:     skillParams,
		Visibility:     "team",
		Status:         "published",
		PublishedAt:    &now,
		ReviewRequired: 1,
		Version:        1,
	})
	if err != nil {
		t.Fatal(err)
	}

	allowedColl, err := db.CreateCollection(ctx, pool, &db.KnowledgeCollection{
		ScopeID:    allowed.ID,
		OwnerID:    principal.ID,
		Slug:       "allowed-coll-" + uuid.New().String(),
		Name:       "Allowed Collection",
		Visibility: "team",
	})
	if err != nil {
		t.Fatal(err)
	}
	blockedColl, err := db.CreateCollection(ctx, pool, &db.KnowledgeCollection{
		ScopeID:    blocked.ID,
		OwnerID:    principal.ID,
		Slug:       "blocked-coll-" + uuid.New().String(),
		Name:       "Blocked Collection",
		Visibility: "team",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AddCollectionItem(ctx, pool, allowedColl.ID, allowedArtifact.ID, principal.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.AddCollectionItem(ctx, pool, blockedColl.ID, blockedArtifact.ID, principal.ID); err != nil {
		t.Fatal(err)
	}

	rawToken, hashToken, err := auth.GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.CreateToken(ctx, pool, principal.ID, hashToken, "rest-scope-id-authz-token", []uuid.UUID{allowed.ID}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	handler := rest.NewRouter(pool, svc, cfg).Handler()
	srv := httptest.NewServer(handler)
	defer srv.Close()

	authHeader := "Bearer " + rawToken

	type endpointCase struct {
		name        string
		allowedID   string
		blockedID   string
		reqBuilder  func(id string) (*http.Request, error)
		successCode int
	}

	cases := []endpointCase{
		{
			name:        "get memory",
			allowedID:   allowedMemory.ID.String(),
			blockedID:   blockedMemory.ID.String(),
			successCode: http.StatusOK,
			reqBuilder: func(id string) (*http.Request, error) {
				return http.NewRequest(http.MethodGet, srv.URL+"/v1/memories/"+id, nil)
			},
		},
		{
			name:        "patch memory",
			allowedID:   allowedMemory.ID.String(),
			blockedID:   blockedMemory.ID.String(),
			successCode: http.StatusOK,
			reqBuilder: func(id string) (*http.Request, error) {
				body := map[string]any{"content": "updated memory", "importance": 0.6}
				b, _ := json.Marshal(body)
				req, err := http.NewRequest(http.MethodPatch, srv.URL+"/v1/memories/"+id, bytes.NewReader(b))
				if err != nil {
					return nil, err
				}
				req.Header.Set("Content-Type", "application/json")
				return req, nil
			},
		},
		{
			name:        "delete memory",
			allowedID:   allowedMemory.ID.String(),
			blockedID:   blockedMemory.ID.String(),
			successCode: http.StatusOK,
			reqBuilder: func(id string) (*http.Request, error) {
				return http.NewRequest(http.MethodDelete, srv.URL+"/v1/memories/"+id, nil)
			},
		},
		{
			name:        "get knowledge",
			allowedID:   allowedArtifact.ID.String(),
			blockedID:   blockedArtifact.ID.String(),
			successCode: http.StatusOK,
			reqBuilder: func(id string) (*http.Request, error) {
				return http.NewRequest(http.MethodGet, srv.URL+"/v1/knowledge/"+id, nil)
			},
		},
		{
			name:        "patch knowledge",
			allowedID:   allowedDraftArtifact.ID.String(),
			blockedID:   blockedDraftArtifact.ID.String(),
			successCode: http.StatusOK,
			reqBuilder: func(id string) (*http.Request, error) {
				body := map[string]any{"title": "updated title", "content": "updated knowledge"}
				b, _ := json.Marshal(body)
				req, err := http.NewRequest(http.MethodPatch, srv.URL+"/v1/knowledge/"+id, bytes.NewReader(b))
				if err != nil {
					return nil, err
				}
				req.Header.Set("Content-Type", "application/json")
				return req, nil
			},
		},
		{
			name:        "get skill",
			allowedID:   allowedSkill.ID.String(),
			blockedID:   blockedSkill.ID.String(),
			successCode: http.StatusOK,
			reqBuilder: func(id string) (*http.Request, error) {
				return http.NewRequest(http.MethodGet, srv.URL+"/v1/skills/"+id, nil)
			},
		},
		{
			name:        "patch skill",
			allowedID:   allowedSkill.ID.String(),
			blockedID:   blockedSkill.ID.String(),
			successCode: http.StatusOK,
			reqBuilder: func(id string) (*http.Request, error) {
				body := map[string]any{"body": "updated skill body", "parameters": []map[string]any{}}
				b, _ := json.Marshal(body)
				req, err := http.NewRequest(http.MethodPatch, srv.URL+"/v1/skills/"+id, bytes.NewReader(b))
				if err != nil {
					return nil, err
				}
				req.Header.Set("Content-Type", "application/json")
				return req, nil
			},
		},
		{
			name:        "get collection by id",
			allowedID:   allowedColl.ID.String(),
			blockedID:   blockedColl.ID.String(),
			successCode: http.StatusOK,
			reqBuilder: func(id string) (*http.Request, error) {
				return http.NewRequest(http.MethodGet, srv.URL+"/v1/collections/"+id, nil)
			},
		},
		{
			name:        "add collection item",
			allowedID:   allowedColl.ID.String(),
			blockedID:   blockedColl.ID.String(),
			successCode: http.StatusCreated,
			reqBuilder: func(id string) (*http.Request, error) {
				body := map[string]any{"artifact_id": allowedArtifact.ID.String()}
				b, _ := json.Marshal(body)
				req, err := http.NewRequest(http.MethodPost, srv.URL+"/v1/collections/"+id+"/items", bytes.NewReader(b))
				if err != nil {
					return nil, err
				}
				req.Header.Set("Content-Type", "application/json")
				return req, nil
			},
		},
		{
			name:        "remove collection item",
			allowedID:   allowedColl.ID.String(),
			blockedID:   blockedColl.ID.String(),
			successCode: http.StatusNoContent,
			reqBuilder: func(id string) (*http.Request, error) {
				return http.NewRequest(http.MethodDelete, srv.URL+"/v1/collections/"+id+"/items/"+allowedArtifact.ID.String(), nil)
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name+" in-scope id succeeds", func(t *testing.T) {
			req, err := tc.reqBuilder(tc.allowedID)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", authHeader)

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tc.successCode {
				t.Fatalf("%s status = %d, want %d", tc.name, resp.StatusCode, tc.successCode)
			}
		})

		t.Run(tc.name+" out-of-scope id is forbidden", func(t *testing.T) {
			req, err := tc.reqBuilder(tc.blockedID)
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
				t.Fatalf("%s status = %d, want %d", tc.name, resp.StatusCode, http.StatusForbidden)
			}
			var out map[string]any
			_ = json.NewDecoder(resp.Body).Decode(&out)
			errMsg, _ := out["error"].(string)
			if !strings.Contains(errMsg, "scope access denied") {
				t.Fatalf("%s error = %q, want scope access denied", tc.name, errMsg)
			}
		})
	}
}

func TestREST_ScopeAuthz_MultiHopPrincipalChain(t *testing.T) {
	for _, role := range []string{"member", "owner", "admin"} {
		role := role
		t.Run("role="+role, func(t *testing.T) {
			ctx := context.Background()
			pool := testhelper.NewTestPool(t)
			svc := testhelper.NewMockEmbeddingService()
			cfg := &config.Config{}

			user := testhelper.CreateTestPrincipal(t, pool, "user", "chain-user-"+role+"-"+uuid.New().String())
			team := testhelper.CreateTestPrincipal(t, pool, "team", "chain-team-"+role+"-"+uuid.New().String())
			company := testhelper.CreateTestPrincipal(t, pool, "company", "chain-company-"+role+"-"+uuid.New().String())
			outsider := testhelper.CreateTestPrincipal(t, pool, "team", "chain-outsider-"+role+"-"+uuid.New().String())

			userScope := testhelper.CreateTestScope(t, pool, "project", "chain-user-scope-"+role+"-"+uuid.New().String(), nil, user.ID)
			teamScope := testhelper.CreateTestScope(t, pool, "project", "chain-team-scope-"+role+"-"+uuid.New().String(), nil, team.ID)
			companyScope := testhelper.CreateTestScope(t, pool, "project", "chain-company-scope-"+role+"-"+uuid.New().String(), nil, company.ID)
			outsiderScope := testhelper.CreateTestScope(t, pool, "project", "chain-outsider-scope-"+role+"-"+uuid.New().String(), nil, outsider.ID)

			ms := principals.NewMembershipStore(pool)
			if err := ms.AddMembership(ctx, user.ID, team.ID, role, nil); err != nil {
				t.Fatalf("add membership user->team (%s): %v", role, err)
			}
			if err := ms.AddMembership(ctx, team.ID, company.ID, role, nil); err != nil {
				t.Fatalf("add membership team->company (%s): %v", role, err)
			}

			handler := rest.NewRouter(pool, svc, cfg).Handler()
			srv := httptest.NewServer(handler)
			defer srv.Close()

			type principalCase struct {
				name         string
				principalID  uuid.UUID
				allowedScope []string
				deniedScope  []string
			}
			cases := []principalCase{
				{
					name:        "user",
					principalID: user.ID,
					allowedScope: []string{
						"project:" + userScope.ExternalID,
						"project:" + teamScope.ExternalID,
						"project:" + companyScope.ExternalID,
					},
					deniedScope: []string{
						"project:" + outsiderScope.ExternalID,
					},
				},
				{
					name:        "team",
					principalID: team.ID,
					allowedScope: []string{
						"project:" + teamScope.ExternalID,
						"project:" + companyScope.ExternalID,
					},
					deniedScope: []string{
						"project:" + userScope.ExternalID, // descendant
						"project:" + outsiderScope.ExternalID,
					},
				},
				{
					name:        "company",
					principalID: company.ID,
					allowedScope: []string{
						"project:" + companyScope.ExternalID,
					},
					deniedScope: []string{
						"project:" + teamScope.ExternalID, // descendant
						"project:" + userScope.ExternalID, // descendant
						"project:" + outsiderScope.ExternalID,
					},
				},
			}

			for _, tc := range cases {
				tc := tc
				t.Run(tc.name+" principal", func(t *testing.T) {
					rawToken, hashToken, err := auth.GenerateToken()
					if err != nil {
						t.Fatal(err)
					}
					_, err = db.CreateToken(ctx, pool, tc.principalID, hashToken, "chain-token-"+tc.name+"-"+uuid.New().String(), nil, nil, nil)
					if err != nil {
						t.Fatal(err)
					}
					authHeader := "Bearer " + rawToken

					for _, scopeStr := range tc.allowedScope {
						scopeStr := scopeStr
						t.Run("allow "+scopeStr, func(t *testing.T) {
							body := map[string]any{"scope": scopeStr}
							b, _ := json.Marshal(body)
							req, err := http.NewRequest(http.MethodPost, srv.URL+"/v1/sessions", bytes.NewReader(b))
							if err != nil {
								t.Fatal(err)
							}
							req.Header.Set("Content-Type", "application/json")
							req.Header.Set("Authorization", authHeader)

							resp, err := http.DefaultClient.Do(req)
							if err != nil {
								t.Fatal(err)
							}
							defer resp.Body.Close()
							if resp.StatusCode != http.StatusCreated {
								t.Fatalf("scope %s status = %d, want %d", scopeStr, resp.StatusCode, http.StatusCreated)
							}
						})
					}

					for _, scopeStr := range tc.deniedScope {
						scopeStr := scopeStr
						t.Run("deny "+scopeStr, func(t *testing.T) {
							body := map[string]any{"scope": scopeStr}
							b, _ := json.Marshal(body)
							req, err := http.NewRequest(http.MethodPost, srv.URL+"/v1/sessions", bytes.NewReader(b))
							if err != nil {
								t.Fatal(err)
							}
							req.Header.Set("Content-Type", "application/json")
							req.Header.Set("Authorization", authHeader)

							resp, err := http.DefaultClient.Do(req)
							if err != nil {
								t.Fatal(err)
							}
							defer resp.Body.Close()
							if resp.StatusCode != http.StatusForbidden {
								t.Fatalf("scope %s status = %d, want %d", scopeStr, resp.StatusCode, http.StatusForbidden)
							}
							var out map[string]any
							_ = json.NewDecoder(resp.Body).Decode(&out)
							errMsg, _ := out["error"].(string)
							if !strings.Contains(errMsg, "scope access denied") {
								t.Fatalf("scope %s error = %q, want scope access denied", scopeStr, errMsg)
							}
						})
					}
				})
			}
		})
	}
}

func TestREST_Recall_IntersectsFanOutWithPrincipalScopes(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	cfg := &config.Config{}
	testhelper.CreateTestEmbeddingModel(t, pool)

	user := testhelper.CreateTestPrincipal(t, pool, "user", "fanout-intersect-user-"+uuid.New().String())
	team := testhelper.CreateTestPrincipal(t, pool, "team", "fanout-intersect-team-"+uuid.New().String())
	company := testhelper.CreateTestPrincipal(t, pool, "company", "fanout-intersect-company-"+uuid.New().String())

	companyScope := testhelper.CreateTestScope(t, pool, "project", "fanout-intersect-company-scope-"+uuid.New().String(), nil, company.ID)
	teamScope := testhelper.CreateTestScope(t, pool, "project", "fanout-intersect-team-scope-"+uuid.New().String(), &companyScope.ID, team.ID)

	ms := principals.NewMembershipStore(pool)
	if err := ms.AddMembership(ctx, user.ID, team.ID, "member", nil); err != nil {
		t.Fatalf("add membership user->team: %v", err)
	}

	memStore := memory.NewStore(pool, svc)
	if _, err := memStore.Create(ctx, memory.CreateInput{
		Content:    "ancestor confidential recall marker",
		MemoryType: "semantic",
		ScopeID:    companyScope.ID,
		AuthorID:   company.ID,
	}); err != nil {
		t.Fatalf("create company memory: %v", err)
	}
	teamMemRes, err := memStore.Create(ctx, memory.CreateInput{
		Content:    "team public recall marker",
		MemoryType: "semantic",
		ScopeID:    teamScope.ID,
		AuthorID:   team.ID,
	})
	if err != nil {
		t.Fatalf("create team memory: %v", err)
	}

	rawToken, hashToken, err := auth.GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.CreateToken(ctx, pool, user.ID, hashToken, "fanout-intersect-token", nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	handler := rest.NewRouter(pool, svc, cfg).Handler()
	srv := httptest.NewServer(handler)
	defer srv.Close()

	req, err := http.NewRequest(
		http.MethodGet,
		srv.URL+"/v1/memories/recall?q=recall+marker&scope=project:"+teamScope.ExternalID,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+rawToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	resultsAny, _ := out["results"].([]any)
	if len(resultsAny) == 0 {
		t.Fatal("expected non-empty recall results")
	}

	foundTeamMemory := false
	for _, item := range resultsAny {
		row, _ := item.(map[string]any)
		memObj, _ := row["Memory"].(map[string]any)
		if memObj == nil {
			memObj, _ = row["memory"].(map[string]any)
		}
		if memObj == nil {
			continue
		}
		scopeID, _ := memObj["ScopeID"].(string)
		if scopeID == "" {
			scopeID, _ = memObj["scope_id"].(string)
		}
		if scopeID == companyScope.ID.String() {
			t.Fatalf("unexpected ancestor-scope memory leaked into recall results: scope_id=%s", scopeID)
		}
		memID, _ := memObj["ID"].(string)
		if memID == "" {
			memID, _ = memObj["id"].(string)
		}
		if memID == teamMemRes.MemoryID.String() {
			foundTeamMemory = true
		}
	}
	if !foundTeamMemory {
		t.Fatalf("expected team-scope memory %s in results", teamMemRes.MemoryID)
	}
}

func TestREST_ScopeAuthz_WriteEndpoints_MultiHopChainMatrix(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	cfg := &config.Config{}
	testhelper.CreateTestEmbeddingModel(t, pool)

	graph := testhelper.CreateScopeAuthzGraph(t, pool, "rest-scope-multihop", "member")

	rawToken, hashToken, err := auth.GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	// nil scope_ids keeps token unrestricted so this test exercises principal-chain authz.
	_, err = db.CreateToken(ctx, pool, graph.UserPrincipal.ID, hashToken, "rest-scope-multihop-token", nil, nil, nil)
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
					"content":     "chain matrix memory content",
					"scope":       scopeStr,
					"memory_type": "semantic",
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
					"title":          "chain matrix artifact",
					"content":        "chain matrix content",
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
					"slug":  "chain-matrix-skill-" + uuid.New().String(),
					"name":  "Chain Matrix Skill",
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
					"slug":  "chain-matrix-coll-" + uuid.New().String(),
					"name":  "Chain Matrix Collection",
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

	allowedScopes := []string{
		"project:" + graph.UserScope.ExternalID,
		"project:" + graph.TeamScope.ExternalID,
		"project:" + graph.CompanyScope.ExternalID,
	}
	deniedScope := "project:" + graph.UnrelatedScope.ExternalID

	for _, tc := range cases {
		tc := tc
		for _, scopeStr := range allowedScopes {
			scopeStr := scopeStr
			t.Run(tc.name+" allows "+scopeStr, func(t *testing.T) {
				req, err := tc.reqBuilder(scopeStr)
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
		}

		t.Run(tc.name+" denies unrelated branch", func(t *testing.T) {
			req, err := tc.reqBuilder(deniedScope)
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
	}
}
