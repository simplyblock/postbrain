// Package ui provides the HTTP handler for the Postbrain Web UI.
package ui

import (
	"context"
	"embed"
	"encoding/json"
	"html/template"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/authz"
	"github.com/simplyblock/postbrain/internal/closeutil"
	"github.com/simplyblock/postbrain/internal/codegraph"
	"github.com/simplyblock/postbrain/internal/config"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/embedding"
	"github.com/simplyblock/postbrain/internal/ingest"
	"github.com/simplyblock/postbrain/internal/knowledge"
	"github.com/simplyblock/postbrain/internal/memory"
	"github.com/simplyblock/postbrain/internal/oauth"
	"github.com/simplyblock/postbrain/internal/principals"
	"github.com/simplyblock/postbrain/internal/retrieval"
	"github.com/simplyblock/postbrain/internal/social"
)

//go:embed web/templates
var templatesFS embed.FS

//go:embed web/static
var staticFS embed.FS

// Handler holds all dependencies for the UI.
type Handler struct {
	pool       *pgxpool.Pool
	templates  *template.Template
	staticFS   fs.FS
	knwLife    *knowledge.Lifecycle
	knwStore   *knowledge.Store
	knwProm    *knowledge.Promoter
	memStore   *memory.Store
	svc        *embedding.EmbeddingService
	syncer     *codegraph.Syncer
	oauthCfg   config.OAuthConfig
	providers  map[string]social.Provider
	stateStore oauthStateStore
	codeStore  oauthCodeStore
	clients    oauthClientLookup
	issuer     oauthIssuer
	identities socialIdentityStore
}

// NewHandler creates a UI Handler with parsed templates.
func NewHandler(pool *pgxpool.Pool, svc *embedding.EmbeddingService) (*Handler, error) {
	funcMap := template.FuncMap{
		"truncate":    truncate,
		"timeAgo":     timeAgo,
		"deref":       derefString,
		"derefTime":   derefTime,
		"statusBadge": statusBadge,
		"join":        strings.Join,
		"add1":        func(i int) int { return i + 1 },
		"tokenHasScope": func(tok *db.Token, scopeID uuid.UUID) bool {
			if tok == nil {
				return false
			}
			for _, id := range tok.ScopeIds {
				if id == scopeID {
					return true
				}
			}
			return false
		},
		"slice": func(s string, i, j int) string {
			if j > len(s) {
				j = len(s)
			}
			return s[i:j]
		},
	}
	tmpl, err := template.New("").Funcs(funcMap).ParseFS(templatesFS, "web/templates/*.html")
	if err != nil {
		return nil, err
	}
	sub, err := fs.Sub(staticFS, "web/static")
	if err != nil {
		return nil, err
	}
	h := &Handler{pool: pool, templates: tmpl, staticFS: sub, syncer: codegraph.NewSyncer(), providers: map[string]social.Provider{}}
	if pool != nil {
		membership := principals.NewMembershipStore(pool)
		h.knwLife = knowledge.NewLifecycle(pool, membership)
		h.knwProm = knowledge.NewPromoter(pool, svc)
	}
	if pool != nil && svc != nil {
		h.knwStore = knowledge.NewStore(pool, svc)
		h.memStore = memory.NewStore(pool, svc)
		h.svc = svc
	}
	return h, nil
}

// NewHandlerWithOAuth creates a UI handler with OAuth/social dependencies wired.
func NewHandlerWithOAuth(
	pool *pgxpool.Pool,
	svc *embedding.EmbeddingService,
	oauthCfg config.OAuthConfig,
	providers map[string]social.Provider,
	stateStore *oauth.StateStore,
	clientStore *oauth.ClientStore,
	codeStore *oauth.CodeStore,
	issuer *oauth.Issuer,
	identities *social.IdentityStore,
) (*Handler, error) {
	h, err := NewHandler(pool, svc)
	if err != nil {
		return nil, err
	}
	h.oauthCfg = oauthCfg
	h.providers = providers
	h.stateStore = stateStore
	h.clients = clientStore
	h.codeStore = codeStore
	h.issuer = issuer
	h.identities = identities
	return h, nil
}

// ServeHTTP routes /ui/* requests.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Serve static files.
	if strings.HasPrefix(r.URL.Path, "/ui/static/") {
		http.StripPrefix("/ui/static/", http.FileServer(http.FS(h.staticFS))).ServeHTTP(w, r)
		return
	}
	// Login form and POST.
	if r.URL.Path == "/ui/login" {
		h.handleLogin(w, r)
		return
	}
	// Social login routes are unauthenticated.
	if strings.HasPrefix(r.URL.Path, "/ui/auth/") && r.Method == http.MethodGet {
		if strings.HasSuffix(r.URL.Path, "/callback") {
			h.handleSocialCallback(w, r)
			return
		}
		h.handleSocialStart(w, r)
		return
	}
	if r.URL.Path == "/ui/oauth/authorize" && r.Method == http.MethodGet {
		next := "/ui/oauth/authorize"
		if raw := r.URL.RawQuery; raw != "" {
			next += "?" + raw
		}
		if !h.authenticatedRedirect(w, r, "/ui/login?next="+url.QueryEscape(next)) {
			return
		}
		h.handleConsentGet(w, r)
		return
	}
	// Auth check for all other routes.
	if !h.authenticated(w, r) {
		return
	}
	if required := requiredUIPermission(r); required != permissionNone && !h.hasUIPermission(r, required) {
		http.Error(w, "forbidden: insufficient permissions", http.StatusForbidden)
		return
	}
	if requiresPrincipalAdminSection(r) && !h.hasAnyPrincipalAdminRole(r.Context(), r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	switch {
	case r.URL.Path == "/ui" || r.URL.Path == "/ui/":
		h.handleOverview(w, r)
	case r.URL.Path == "/ui/memories":
		h.handleMemories(w, r)
	case strings.HasPrefix(r.URL.Path, "/ui/memories/"):
		h.handleMemoryDetail(w, r)
	case r.URL.Path == "/ui/knowledge/upload" && r.Method == http.MethodPost:
		h.handleUploadKnowledge(w, r)
	case r.URL.Path == "/ui/knowledge/new":
		h.handleKnowledgeNew(w, r)
	case r.URL.Path == "/ui/knowledge" && r.Method == http.MethodPost:
		h.handleCreateKnowledge(w, r)
	case r.URL.Path == "/ui/knowledge":
		h.handleKnowledge(w, r)
	case strings.HasSuffix(r.URL.Path, "/endorse") && strings.HasPrefix(r.URL.Path, "/ui/knowledge/") && r.Method == http.MethodPost:
		h.handleEndorseArtifact(w, r)
	case strings.HasSuffix(r.URL.Path, "/review") && strings.HasPrefix(r.URL.Path, "/ui/knowledge/") && r.Method == http.MethodPost:
		h.handleKnowledgeReview(w, r)
	case strings.HasSuffix(r.URL.Path, "/retract") && strings.HasPrefix(r.URL.Path, "/ui/knowledge/") && r.Method == http.MethodPost:
		h.handleKnowledgeRetract(w, r)
	case strings.HasSuffix(r.URL.Path, "/deprecate") && strings.HasPrefix(r.URL.Path, "/ui/knowledge/") && r.Method == http.MethodPost:
		h.handleKnowledgeDeprecate(w, r)
	case strings.HasSuffix(r.URL.Path, "/republish") && strings.HasPrefix(r.URL.Path, "/ui/knowledge/") && r.Method == http.MethodPost:
		h.handleKnowledgeRepublish(w, r)
	case strings.HasSuffix(r.URL.Path, "/delete") && strings.HasPrefix(r.URL.Path, "/ui/knowledge/") && r.Method == http.MethodPost:
		h.handleKnowledgeDelete(w, r)
	case strings.HasSuffix(r.URL.Path, "/history") && strings.HasPrefix(r.URL.Path, "/ui/knowledge/"):
		h.handleKnowledgeHistory(w, r)
	case strings.HasPrefix(r.URL.Path, "/ui/knowledge/"):
		h.handleKnowledgeDetail(w, r)
	case r.URL.Path == "/ui/collections" && r.Method == http.MethodPost:
		h.handleCreateCollection(w, r)
	case r.URL.Path == "/ui/collections":
		h.handleCollections(w, r)
	case r.URL.Path == "/ui/collections/new":
		h.handleCollectionNew(w, r)
	case strings.HasSuffix(r.URL.Path, "/forget") && strings.HasPrefix(r.URL.Path, "/ui/memories/") && r.Method == http.MethodPost:
		h.handleMemoryForget(w, r)
	case strings.HasPrefix(r.URL.Path, "/ui/collections/"):
		h.handleCollectionDetail(w, r)
	case r.URL.Path == "/ui/promotions":
		h.handlePromotions(w, r)
	case strings.HasSuffix(r.URL.Path, "/approve") && strings.HasPrefix(r.URL.Path, "/ui/promotions/") && r.Method == http.MethodPost:
		h.handleApprovePromotion(w, r)
	case strings.HasSuffix(r.URL.Path, "/reject") && strings.HasPrefix(r.URL.Path, "/ui/promotions/") && r.Method == http.MethodPost:
		h.handleRejectPromotion(w, r)
	case r.URL.Path == "/ui/staleness":
		h.handleStaleness(w, r)
	case r.URL.Path == "/ui/graph":
		h.handleGraph(w, r)
	case r.URL.Path == "/ui/graph3d":
		h.handleGraph3D(w, r)
	case r.URL.Path == "/ui/skills":
		h.handleSkills(w, r)
	case strings.HasSuffix(r.URL.Path, "/history") && strings.HasPrefix(r.URL.Path, "/ui/skills/"):
		h.handleSkillHistory(w, r)
	case strings.HasPrefix(r.URL.Path, "/ui/skills/"):
		h.handleSkillDetail(w, r)
	case strings.HasPrefix(r.URL.Path, "/ui/principals/") && r.Method == http.MethodPost:
		h.handleUpdatePrincipal(w, r)
	case r.URL.Path == "/ui/principals" && r.Method == http.MethodPost:
		h.handleCreatePrincipal(w, r)
	case r.URL.Path == "/ui/principals":
		h.handlePrincipals(w, r)
	case r.URL.Path == "/ui/scopes" && r.Method == http.MethodPost:
		h.handleCreateScope(w, r)
	case r.URL.Path == "/ui/scopes":
		h.handleScopes(w, r)
	case strings.HasSuffix(r.URL.Path, "/delete") && strings.HasPrefix(r.URL.Path, "/ui/scopes/") && r.Method == http.MethodPost:
		h.handleDeleteScope(w, r)
	case strings.HasSuffix(r.URL.Path, "/repo/sync/status") && strings.HasPrefix(r.URL.Path, "/ui/scopes/") && r.Method == http.MethodGet:
		h.handleSyncStatus(w, r)
	case strings.HasSuffix(r.URL.Path, "/repo/sync") && strings.HasPrefix(r.URL.Path, "/ui/scopes/") && r.Method == http.MethodPost:
		h.handleSyncScopeRepo(w, r)
	case strings.HasSuffix(r.URL.Path, "/repo") && strings.HasPrefix(r.URL.Path, "/ui/scopes/") && r.Method == http.MethodPost:
		h.handleSetScopeRepo(w, r)
	case strings.HasSuffix(r.URL.Path, "/owner") && strings.HasPrefix(r.URL.Path, "/ui/scopes/") && r.Method == http.MethodPost:
		h.handleSetScopeOwner(w, r)
	case r.URL.Path == "/ui/oauth/authorize" && r.Method == http.MethodPost:
		h.handleConsentPost(w, r)
	case r.URL.Path == "/ui/memberships" && r.Method == http.MethodPost:
		h.handleAddMembership(w, r)
	case r.URL.Path == "/ui/memberships/delete" && r.Method == http.MethodPost:
		h.handleDeleteMembership(w, r)
	case r.URL.Path == "/ui/tokens" && r.Method == http.MethodPost:
		h.handleCreateToken(w, r)
	case strings.HasSuffix(r.URL.Path, "/scopes") && strings.HasPrefix(r.URL.Path, "/ui/tokens/") && r.Method == http.MethodPost:
		h.handleUpdateTokenScopes(w, r)
	case strings.HasSuffix(r.URL.Path, "/revoke") && strings.HasPrefix(r.URL.Path, "/ui/tokens/") && r.Method == http.MethodPost:
		h.handleRevokeToken(w, r)
	case r.URL.Path == "/ui/tokens":
		h.handleTokens(w, r)
	case r.URL.Path == "/ui/logout" && r.Method == http.MethodPost:
		h.handleLogout(w, r)
	case r.URL.Path == "/ui/metrics":
		h.handleMetrics(w, r)
	case r.URL.Path == "/ui/query":
		h.handleQuery(w, r)
	default:
		http.NotFound(w, r)
	}
}

type uiPermission string

const (
	permissionNone  uiPermission = "none"
	permissionRead  uiPermission = "read"
	permissionWrite uiPermission = "write"
)

func requiredUIPermission(r *http.Request) uiPermission {
	if r.URL.Path == "/ui/logout" && r.Method == http.MethodPost {
		return permissionNone
	}
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return permissionRead
	default:
		return permissionWrite
	}
}

func (h *Handler) hasUIPermission(r *http.Request, required uiPermission) bool {
	token := h.tokenFromCookie(r)
	if token == nil {
		return false
	}
	if required == permissionNone {
		return true
	}
	perms, err := authz.ParseTokenPermissions(token.Permissions)
	if err != nil {
		return false
	}
	switch required {
	case permissionRead:
		// Allow if the token holds any :read permission.
		for _, p := range perms.Permissions() {
			_, op, err := p.Parse()
			if err == nil && op == authz.OperationRead {
				return true
			}
		}
		return false
	case permissionWrite:
		// Allow if the token holds any :write permission.
		for _, p := range perms.Permissions() {
			_, op, err := p.Parse()
			if err == nil && op == authz.OperationWrite {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// render renders a named template wrapped in the base layout.
// For HTMX requests (HX-Request: true header), renders only the content template.
func (h *Handler) render(w http.ResponseWriter, r *http.Request, tmplName string, title string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if r.Header.Get("HX-Request") == "true" {
		if err := h.templates.ExecuteTemplate(w, tmplName, data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	// Render content to buffer first, then wrap in base layout.
	var buf strings.Builder
	if err := h.templates.ExecuteTemplate(&buf, tmplName, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	layout := "base"
	if tmplName == "login" {
		layout = "auth_base"
	}
	if err := h.templates.ExecuteTemplate(w, layout, struct {
		Title          string
		Content        template.HTML
		ShowPrincipals bool
	}{
		Title:          title,
		Content:        template.HTML(buf.String()), //nolint:gosec // generated by our own templates
		ShowPrincipals: h.hasAnyPrincipalAdminRole(r.Context(), r),
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func requiresPrincipalAdminSection(r *http.Request) bool {
	if strings.HasPrefix(r.URL.Path, "/ui/principals") {
		return true
	}
	return r.URL.Path == "/ui/memberships" || r.URL.Path == "/ui/memberships/delete"
}

// handleOverview serves GET /ui — server health and schema version.
func (h *Handler) handleOverview(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Status          string
		SchemaVersion   uint
		ExpectedVersion uint
		SchemaDirty     bool
	}{
		Status:          "ok",
		SchemaVersion:   0,
		ExpectedVersion: 0,
		SchemaDirty:     false,
	}
	if h.pool != nil {
		sv, dirty, err := db.SchemaVersion(r.Context(), h.pool)
		if err == nil {
			data.SchemaVersion = sv
			data.SchemaDirty = dirty
		} else {
			data.Status = "degraded"
		}
	}
	h.render(w, r, "health", "Overview", data)
}

const memoriesPageSize = 50

// handleMemories serves GET /ui/memories.
func (h *Handler) handleMemories(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	scopeID := r.URL.Query().Get("scope_id")
	offset := 0
	if c := r.URL.Query().Get("cursor"); c != "" {
		if v, err := strconv.Atoi(c); err == nil && v > 0 {
			offset = v
		}
	}

	data := struct {
		Query      string
		ScopeID    string
		Scopes     []*db.Scope
		Memories   []*db.Memory
		NextCursor string
	}{
		Query:   q,
		ScopeID: scopeID,
	}

	if h.pool != nil {
		scopes, scopeSet := h.authorizedScopesForRequest(r.Context(), r)
		data.Scopes = scopes
		if scopeID == "" && len(data.Scopes) > 0 {
			scopeID = data.Scopes[0].ID.String()
			data.ScopeID = scopeID
		}
		if scopeID != "" {
			if sid, err := uuid.Parse(scopeID); err == nil {
				if _, ok := scopeSet[sid]; !ok {
					goto doneMemories
				}
				mems, err := db.ListMemoriesByScope(r.Context(), h.pool, sid, memoriesPageSize+1, offset)
				if err == nil {
					if len(mems) > memoriesPageSize {
						data.Memories = mems[:memoriesPageSize]
						data.NextCursor = strconv.Itoa(offset + memoriesPageSize)
					} else {
						data.Memories = mems
					}
				}
			}
		}
	}
doneMemories:

	tmpl := "memories"
	if r.Header.Get("HX-Request") == "true" {
		tmpl = "memories_rows"
	}
	h.render(w, r, tmpl, "Memories", data)
}

// handleMemoryDetail serves GET /ui/memories/{id}.
func (h *Handler) handleMemoryDetail(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/ui/memories/")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if h.pool == nil {
		http.NotFound(w, r)
		return
	}

	mem, err := db.GetMemory(r.Context(), h.pool, id)
	if err != nil || mem == nil {
		http.NotFound(w, r)
		return
	}

	h.render(w, r, "memory_detail", "Memory", struct{ Memory *db.Memory }{mem})
}

// handleKnowledge serves GET /ui/knowledge.
const knowledgePageSize = 50

func (h *Handler) handleKnowledge(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	status := r.URL.Query().Get("status")
	scopeStr := r.URL.Query().Get("scope")

	cursor := 0
	if c, err := strconv.Atoi(r.URL.Query().Get("cursor")); err == nil && c > 0 {
		cursor = c
	}

	var scopeID uuid.UUID
	if scopeStr != "" {
		if id, err := uuid.Parse(scopeStr); err == nil {
			scopeID = id
		}
	}

	data := struct {
		Query       string
		Status      string
		ScopeID     uuid.UUID
		Artifacts   []*db.KnowledgeArtifact
		Scopes      []*db.Scope
		UploadError string
		PrevCursor  int
		NextCursor  int
		HasPrev     bool
		HasNext     bool
	}{
		Query:      q,
		Status:     status,
		ScopeID:    scopeID,
		PrevCursor: cursor - knowledgePageSize,
		HasPrev:    cursor > 0,
	}

	if h.pool != nil {
		scopes, scopeSet := h.authorizedScopesForRequest(r.Context(), r)
		data.Scopes = scopes
		scopeAllowed := scopeID == uuid.Nil
		if scopeID != uuid.Nil {
			_, scopeAllowed = scopeSet[scopeID]
		}
		var arts []*db.KnowledgeArtifact
		var err error
		if scopeAllowed {
			if q != "" {
				arts, err = db.SearchArtifacts(r.Context(), h.pool, q, status, scopeID, knowledgePageSize+1, cursor)
			} else if status != "" {
				arts, err = db.ListArtifactsByStatus(r.Context(), h.pool, status, scopeID, knowledgePageSize+1, cursor)
			} else {
				arts, err = db.ListAllArtifacts(r.Context(), h.pool, scopeID, knowledgePageSize+1, cursor)
			}
			if err == nil {
				filtered := make([]*db.KnowledgeArtifact, 0, len(arts))
				for _, art := range arts {
					if _, ok := scopeSet[art.OwnerScopeID]; ok {
						filtered = append(filtered, art)
					}
				}
				if len(filtered) > knowledgePageSize {
					data.Artifacts = filtered[:knowledgePageSize]
					data.HasNext = true
					data.NextCursor = cursor + knowledgePageSize
				} else {
					data.Artifacts = filtered
				}
			}
		}
	}

	tmpl := "knowledge"
	if r.Header.Get("HX-Request") == "true" {
		tmpl = "knowledge_rows"
	}
	h.render(w, r, tmpl, "Knowledge", data)
}

// handleKnowledgeDetail serves GET /ui/knowledge/{id}.
func (h *Handler) handleKnowledgeDetail(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/ui/knowledge/")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if h.pool == nil {
		http.NotFound(w, r)
		return
	}

	data := struct {
		Artifact *db.KnowledgeArtifact
		Sources  []*db.KnowledgeArtifact // populated when Artifact is a digest
		Digests  []*db.KnowledgeArtifact // published digests covering this artifact
	}{}

	if h.pool != nil {
		art, err := db.GetArtifact(r.Context(), h.pool, id)
		if err != nil || art == nil {
			http.NotFound(w, r)
			return
		}
		data.Artifact = art

		if art.KnowledgeType == "digest" {
			sources, err := db.ListDigestSources(r.Context(), h.pool, id)
			if err != nil {
				http.Error(w, "failed to load digest sources", http.StatusInternalServerError)
				return
			}
			data.Sources = sources
		} else {
			digests, err := db.ListDigestsForSource(r.Context(), h.pool, id)
			if err != nil {
				http.Error(w, "failed to load digests", http.StatusInternalServerError)
				return
			}
			data.Digests = digests
		}
	}

	h.render(w, r, "knowledge_detail", "Knowledge", data)
}

// handleKnowledgeHistory serves GET /ui/knowledge/{id}/history.
func (h *Handler) handleKnowledgeHistory(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/ui/knowledge/"), "/history")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	data := struct {
		Artifact *db.KnowledgeArtifact
		History  []*db.KnowledgeHistory
	}{}

	if h.pool != nil {
		art, err := db.GetArtifact(r.Context(), h.pool, id)
		if err != nil || art == nil {
			http.NotFound(w, r)
			return
		}
		data.Artifact = art
		history, _ := db.GetArtifactHistory(r.Context(), h.pool, id)
		data.History = history
	}

	h.render(w, r, "knowledge_history", "Knowledge History", data)
}

// handleCollections serves GET /ui/collections.
func (h *Handler) handleCollections(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Collections []*db.KnowledgeCollection
	}{}

	if h.pool != nil {
		_, scopeSet := h.authorizedScopesForRequest(r.Context(), r)
		var colls []*db.KnowledgeCollection
		var err error
		scopeStr := r.URL.Query().Get("scope_id")
		if scopeStr != "" {
			sid, parseErr := uuid.Parse(scopeStr)
			if parseErr == nil {
				if _, ok := scopeSet[sid]; !ok {
					data.Collections = []*db.KnowledgeCollection{}
					h.render(w, r, "collections", "Collections", data)
					return
				}
				colls, err = db.ListCollections(r.Context(), h.pool, sid)
			}
		} else {
			colls, err = db.ListAllCollections(r.Context(), h.pool)
		}
		if err != nil {
			http.Error(w, "failed to load collections", http.StatusInternalServerError)
			return
		}
		filtered := make([]*db.KnowledgeCollection, 0, len(colls))
		for _, c := range colls {
			if _, ok := scopeSet[c.ScopeID]; ok {
				filtered = append(filtered, c)
			}
		}
		data.Collections = filtered
	}

	h.render(w, r, "collections", "Collections", data)
}

// handleCollectionDetail serves GET /ui/collections/{id}.
func (h *Handler) handleCollectionDetail(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/ui/collections/")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if h.pool == nil {
		http.NotFound(w, r)
		return
	}

	coll, err := db.GetCollection(r.Context(), h.pool, id)
	if err != nil || coll == nil {
		http.NotFound(w, r)
		return
	}
	arts, err := db.ListCollectionItems(r.Context(), h.pool, id)
	if err != nil {
		http.Error(w, "failed to load collection items", http.StatusInternalServerError)
		return
	}
	h.render(w, r, "collection_detail", "Collection", struct {
		Collection *db.KnowledgeCollection
		Artifacts  []*db.KnowledgeArtifact
	}{coll, arts})
}

// handlePromotions serves GET /ui/promotions.
func (h *Handler) handlePromotions(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Scopes     []*db.Scope
		ScopeID    string
		Status     string
		Promotions []*db.PromotionRequest
	}{
		ScopeID: r.URL.Query().Get("scope_id"),
		Status:  r.URL.Query().Get("status"),
	}
	if data.Status == "" {
		data.Status = "all"
	}

	targetScopeID := uuid.Nil
	if data.ScopeID != "" {
		parsed, err := uuid.Parse(data.ScopeID)
		if err != nil {
			http.Error(w, "invalid scope id", http.StatusBadRequest)
			return
		}
		targetScopeID = parsed
	}
	validStatus := map[string]bool{
		"all":      true,
		"pending":  true,
		"approved": true,
		"rejected": true,
		"merged":   true,
	}
	if !validStatus[data.Status] {
		http.Error(w, "invalid status", http.StatusBadRequest)
		return
	}

	if h.pool != nil {
		scopes, scopeSet := h.authorizedScopesForRequest(r.Context(), r)
		data.Scopes = scopes
		if targetScopeID != uuid.Nil {
			if _, ok := scopeSet[targetScopeID]; !ok {
				data.Promotions = []*db.PromotionRequest{}
				h.render(w, r, "promotions", "Promotion Queue", data)
				return
			}
		}

		statusFilter := ""
		if data.Status != "all" {
			statusFilter = data.Status
		}
		proms, err := db.ListPromotions(r.Context(), h.pool, targetScopeID, statusFilter, 500)
		if err != nil {
			http.Error(w, "failed to load promotions", http.StatusInternalServerError)
			return
		}
		filtered := make([]*db.PromotionRequest, 0, len(proms))
		for _, p := range proms {
			if _, ok := scopeSet[p.TargetScopeID]; ok {
				filtered = append(filtered, p)
			}
		}
		data.Promotions = filtered
	}

	h.render(w, r, "promotions", "Promotion Queue", data)
}

// handleApprovePromotion serves POST /ui/promotions/{id}/approve.
func (h *Handler) handleApprovePromotion(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/ui/promotions/"), "/approve")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "invalid promotion id", http.StatusBadRequest)
		return
	}
	reviewerID := h.principalFromCookie(r)
	if _, err := h.knwProm.Approve(r.Context(), id, reviewerID, reviewerID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/ui/promotions", http.StatusSeeOther)
}

// handleRejectPromotion serves POST /ui/promotions/{id}/reject.
func (h *Handler) handleRejectPromotion(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/ui/promotions/"), "/reject")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "invalid promotion id", http.StatusBadRequest)
		return
	}
	reviewerID := h.principalFromCookie(r)
	if err := h.knwProm.Reject(r.Context(), id, reviewerID, nil); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/ui/promotions", http.StatusSeeOther)
}

// handleStaleness serves GET /ui/staleness.
func (h *Handler) handleStaleness(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Flags []*db.StalenessFlag
	}{}

	if h.pool != nil {
		_, scopeSet := h.authorizedScopesForRequest(r.Context(), r)
		flags, err := db.ListStalenessFlags(r.Context(), h.pool, "open", 50, 0)
		if err == nil {
			filtered := make([]*db.StalenessFlag, 0, len(flags))
			for _, f := range flags {
				art, getErr := db.GetArtifact(r.Context(), h.pool, f.ArtifactID)
				if getErr != nil || art == nil {
					continue
				}
				if _, ok := scopeSet[art.OwnerScopeID]; ok {
					filtered = append(filtered, f)
				}
			}
			data.Flags = filtered
		}
	}

	h.render(w, r, "staleness", "Staleness Flags", data)
}

// graphNode is the JSON shape consumed by the D3 force simulation.
type graphNode struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// graphLink is the JSON shape for a relation edge.
type graphLink struct {
	Source     string  `json:"source"`
	Target     string  `json:"target"`
	Predicate  string  `json:"predicate"`
	Confidence float64 `json:"confidence"`
}

// handleGraph serves GET /ui/graph.
func (h *Handler) handleGraph(w http.ResponseWriter, r *http.Request) {
	data := h.graphViewData(r, r.URL.Query().Get("scope_id"))
	h.render(w, r, "graph", "Entity Graph", data)
}

// handleGraph3D serves GET /ui/graph3d.
func (h *Handler) handleGraph3D(w http.ResponseWriter, r *http.Request) {
	data := h.graphViewData(r, r.URL.Query().Get("scope_id"))
	h.render(w, r, "graph3d", "Entity Graph 3D", data)
}

func (h *Handler) graphViewData(r *http.Request, scopeStr string) struct {
	Scopes    []*db.Scope
	ScopeID   string
	NodeCount int
	EdgeCount int
	GraphJSON template.JS
} {
	data := struct {
		Scopes    []*db.Scope
		ScopeID   string
		NodeCount int
		EdgeCount int
		GraphJSON template.JS
	}{ScopeID: scopeStr}

	if h.pool == nil {
		return data
	}

	scopes, scopeSet := h.authorizedScopesForRequest(r.Context(), r)
	data.Scopes = scopes

	// Default to first scope when none is selected.
	if scopeStr == "" && len(data.Scopes) > 0 {
		scopeStr = data.Scopes[0].ID.String()
		data.ScopeID = scopeStr
	}
	if scopeStr != "" {
		if sid, err := uuid.Parse(scopeStr); err == nil {
			if _, ok := scopeSet[sid]; !ok {
				if len(data.Scopes) > 0 {
					scopeStr = data.Scopes[0].ID.String()
					data.ScopeID = scopeStr
				} else {
					scopeStr = ""
					data.ScopeID = ""
				}
			}
		}
	}

	if scopeStr != "" {
		sid, err := uuid.Parse(scopeStr)
		if err == nil {
			nodes := []graphNode{}
			links := []graphLink{}

			ents, err := db.ListEntitiesByScope(r.Context(), h.pool, sid, "", 100000, 0)
			if err == nil {
				for _, e := range ents {
					nodes = append(nodes, graphNode{
						ID:   e.ID.String(),
						Name: e.Name,
						Type: e.EntityType,
					})
				}
			}

			nodeIDs := make(map[string]bool, len(nodes))
			for _, n := range nodes {
				nodeIDs[n.ID] = true
			}

			if rels, err := db.ListRelationsByScope(r.Context(), h.pool, sid); err == nil {
				for _, rel := range rels {
					src, tgt := rel.SubjectID.String(), rel.ObjectID.String()
					if !nodeIDs[src] || !nodeIDs[tgt] {
						continue // skip dangling relations
					}
					links = append(links, graphLink{
						Source:     src,
						Target:     tgt,
						Predicate:  rel.Predicate,
						Confidence: rel.Confidence,
					})
				}
			}

			data.NodeCount = len(nodes)
			data.EdgeCount = len(links)

			payload, err := json.Marshal(map[string]any{"nodes": nodes, "links": links})
			if err == nil {
				data.GraphJSON = template.JS(payload)
			}
		}
	}

	return data
}

// handleSkills serves GET /ui/skills.
func (h *Handler) handleSkills(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Skills []*db.Skill
	}{}

	if h.pool != nil {
		scopes, scopeSet := h.authorizedScopesForRequest(r.Context(), r)
		authorizedScopeIDs := make([]uuid.UUID, 0, len(scopes))
		for _, s := range scopes {
			authorizedScopeIDs = append(authorizedScopeIDs, s.ID)
		}
		skills, err := db.ListPublishedSkillsForAgent(r.Context(), h.pool, authorizedScopeIDs, "any")
		if err == nil {
			filtered := make([]*db.Skill, 0, len(skills))
			for _, s := range skills {
				if _, ok := scopeSet[s.ScopeID]; ok {
					filtered = append(filtered, s)
				}
			}
			data.Skills = filtered
		}
	}

	h.render(w, r, "skills", "Skills", data)
}

// handleSkillDetail serves GET /ui/skills/{id}.
func (h *Handler) handleSkillDetail(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/ui/skills/")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	data := struct {
		Skill *db.Skill
	}{}

	if h.pool != nil {
		skill, err := db.GetSkill(r.Context(), h.pool, id)
		if err != nil || skill == nil {
			http.NotFound(w, r)
			return
		}
		data.Skill = skill
	}

	h.render(w, r, "skill_detail", "Skill", data)
}

// handleSkillHistory serves GET /ui/skills/{id}/history.
func (h *Handler) handleSkillHistory(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/ui/skills/"), "/history")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	data := struct {
		Skill   *db.Skill
		History []*db.SkillHistory
	}{}

	if h.pool != nil {
		skill, err := db.GetSkill(r.Context(), h.pool, id)
		if err != nil || skill == nil {
			http.NotFound(w, r)
			return
		}
		data.Skill = skill
		history, _ := db.GetSkillHistory(r.Context(), h.pool, id)
		data.History = history
	}

	h.render(w, r, "skill_history", "Skill History", data)
}

// handlePrincipals serves GET /ui/principals.
func (h *Handler) handlePrincipals(w http.ResponseWriter, r *http.Request) {
	h.renderPrincipals(w, r, "", "", "")
}

// handleScopes serves GET /ui/scopes.
func (h *Handler) handleScopes(w http.ResponseWriter, r *http.Request) {
	h.renderScopes(w, r, "")
}

// renderScopes renders the scopes page with an optional form error.
func (h *Handler) renderScopes(w http.ResponseWriter, r *http.Request, scopeErr string) {
	data := struct {
		Principals     []*db.Principal
		Scopes         []*db.Scope
		ScopeFormError string
		SyncStatus     map[string]codegraph.SyncStatus
		ChildCount     map[string]int64
		CanManage      map[string]bool
		CanDelete      map[string]bool
		OwnerNames     map[string]string
	}{
		ScopeFormError: scopeErr,
		SyncStatus:     make(map[string]codegraph.SyncStatus),
		ChildCount:     make(map[string]int64),
		CanManage:      make(map[string]bool),
		CanDelete:      make(map[string]bool),
		OwnerNames:     make(map[string]string),
	}

	if h.pool != nil {
		scopes, writable := h.authorizedScopesForRequest(r.Context(), r)
		principals, err := db.ListPrincipals(r.Context(), h.pool, 50, 0)
		if err == nil {
			data.Principals = principals
			for _, p := range principals {
				data.OwnerNames[p.ID.String()] = p.DisplayName
			}
		}
		filtered := make([]*db.Scope, 0, len(scopes))
		for _, s := range scopes {
			if _, ok := writable[s.ID]; !ok {
				continue
			}
			filtered = append(filtered, s)
			st := h.syncer.Status(s.ID)
			if st.State != codegraph.SyncIdle || st.CommitSHA != "" || st.Error != "" {
				data.SyncStatus[s.ID.String()] = st
			}
			if n, err := db.CountChildScopes(r.Context(), h.pool, s.ID); err == nil && n > 0 {
				data.ChildCount[s.ID.String()] = n
			}
			canManage := h.hasScopeAdminAccess(r.Context(), r, s.ID)
			data.CanManage[s.ID.String()] = canManage
			data.CanDelete[s.ID.String()] = canManage
		}
		data.Scopes = filtered
	}

	h.render(w, r, "scopes", "Scopes", data)
}

// renderPrincipals renders the principals page with optional form errors.
func (h *Handler) renderPrincipals(w http.ResponseWriter, r *http.Request, principalErr, principalEditErr, membershipErr string) {
	data := struct {
		Principals          []*db.Principal
		Memberships         []*db.MembershipRow
		PrincipalFormError  string
		PrincipalEditError  string
		MembershipFormError string
	}{
		PrincipalFormError:  principalErr,
		PrincipalEditError:  principalEditErr,
		MembershipFormError: membershipErr,
	}

	if h.pool != nil {
		reachable := h.reachablePrincipalIDSet(r.Context(), r)
		principals, err := db.ListPrincipals(r.Context(), h.pool, 50, 0)
		if err == nil {
			filtered := make([]*db.Principal, 0, len(principals))
			for _, p := range principals {
				if _, ok := reachable[p.ID]; !ok {
					continue
				}
				filtered = append(filtered, p)
			}
			data.Principals = filtered
		}
		memberships, err := db.ListAllMemberships(r.Context(), h.pool)
		if err == nil {
			filtered := make([]*db.MembershipRow, 0, len(memberships))
			for _, m := range memberships {
				if _, ok := reachable[m.MemberID]; !ok {
					continue
				}
				if _, ok := reachable[m.ParentID]; !ok {
					continue
				}
				filtered = append(filtered, m)
			}
			data.Memberships = filtered
		}
	}

	h.render(w, r, "principals", "Principals", data)
}

// handleDeleteScope serves POST /ui/scopes/{id}/delete.
func (h *Handler) handleDeleteScope(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimPrefix(r.URL.Path, "/ui/scopes/")
	idStr := strings.TrimSuffix(trimmed, "/delete")
	id, err := uuid.Parse(idStr)
	if err != nil {
		h.renderScopes(w, r, "invalid scope id")
		return
	}
	if h.pool == nil {
		h.renderScopes(w, r, "service unavailable")
		return
	}
	if !h.hasScopeAdminAccess(r.Context(), r, id) {
		h.renderScopes(w, r, "scope admin required")
		return
	}
	children, err := db.CountChildScopes(r.Context(), h.pool, id)
	if err != nil {
		h.renderScopes(w, r, "could not check for child scopes")
		return
	}
	if children > 0 {
		h.renderScopes(w, r, "cannot delete scope: it has child scopes that must be deleted first")
		return
	}
	if err := db.DeleteScope(r.Context(), h.pool, id); err != nil {
		h.renderScopes(w, r, err.Error())
		return
	}
	http.Redirect(w, r, "/ui/scopes", http.StatusSeeOther)
}

// handleSetScopeOwner serves POST /ui/scopes/{id}/owner.
func (h *Handler) handleSetScopeOwner(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimPrefix(r.URL.Path, "/ui/scopes/")
	idStr := strings.TrimSuffix(trimmed, "/owner")
	id, err := uuid.Parse(idStr)
	if err != nil {
		h.renderScopes(w, r, "invalid scope id")
		return
	}
	if err := r.ParseForm(); err != nil {
		h.renderScopes(w, r, "bad form data")
		return
	}
	principalIDStr := r.FormValue("principal_id")
	if principalIDStr == "" {
		h.renderScopes(w, r, "principal_id is required")
		return
	}
	principalID, err := uuid.Parse(principalIDStr)
	if err != nil {
		h.renderScopes(w, r, "invalid principal_id")
		return
	}
	if h.pool == nil {
		h.renderScopes(w, r, "service unavailable")
		return
	}
	if !h.hasScopeAdminAccess(r.Context(), r, id) {
		h.renderScopes(w, r, "scope admin required")
		return
	}
	if _, err := db.UpdateScopeOwner(r.Context(), h.pool, id, principalID); err != nil {
		h.renderScopes(w, r, err.Error())
		return
	}
	http.Redirect(w, r, "/ui/scopes", http.StatusSeeOther)
}

// handleCreatePrincipal serves POST /ui/principals.
func (h *Handler) handleCreatePrincipal(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderPrincipals(w, r, "bad form data", "", "")
		return
	}
	kind := r.FormValue("kind")
	slug := r.FormValue("slug")
	displayName := r.FormValue("display_name")
	if kind == "" || slug == "" || displayName == "" {
		h.renderPrincipals(w, r, "kind, slug and display_name are required", "", "")
		return
	}
	if h.pool == nil {
		h.renderPrincipals(w, r, "service unavailable", "", "")
		return
	}
	if !h.hasAnyPrincipalAdminRole(r.Context(), r) {
		h.renderPrincipals(w, r, "principal admin required", "", "")
		return
	}
	ps := principals.NewStore(h.pool)
	if _, err := ps.Create(r.Context(), kind, slug, displayName, nil); err != nil {
		h.renderPrincipals(w, r, err.Error(), "", "")
		return
	}
	http.Redirect(w, r, "/ui/principals", http.StatusSeeOther)
}

// handleUpdatePrincipal serves POST /ui/principals/{id}.
func (h *Handler) handleUpdatePrincipal(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/ui/principals/")
	id, err := uuid.Parse(idStr)
	if err != nil {
		h.renderPrincipals(w, r, "", "invalid principal id", "")
		return
	}
	if err := r.ParseForm(); err != nil {
		h.renderPrincipals(w, r, "", "bad form data", "")
		return
	}
	slug := r.FormValue("slug")
	displayName := r.FormValue("display_name")
	if slug == "" || displayName == "" {
		h.renderPrincipals(w, r, "", "slug and display_name are required", "")
		return
	}
	if h.pool == nil {
		h.renderPrincipals(w, r, "", "service unavailable", "")
		return
	}
	if !h.hasPrincipalAdminAccess(r.Context(), r, id) {
		h.renderPrincipals(w, r, "", "principal admin required", "")
		return
	}
	ps := principals.NewStore(h.pool)
	if _, err := ps.UpdateProfile(r.Context(), id, slug, displayName); err != nil {
		h.renderPrincipals(w, r, "", err.Error(), "")
		return
	}
	http.Redirect(w, r, "/ui/principals", http.StatusSeeOther)
}

// handleCreateScope serves POST /ui/scopes.
func (h *Handler) handleCreateScope(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderScopes(w, r, "bad form data")
		return
	}
	kind := r.FormValue("kind")
	externalID := r.FormValue("external_id")
	name := r.FormValue("name")
	principalIDStr := r.FormValue("principal_id")
	parentIDStr := r.FormValue("parent_id")

	if kind == "" || externalID == "" || name == "" || principalIDStr == "" {
		h.renderScopes(w, r, "kind, external_id, name and principal are required")
		return
	}
	principalID, err := uuid.Parse(principalIDStr)
	if err != nil {
		h.renderScopes(w, r, "invalid principal id")
		return
	}
	var parentID *uuid.UUID
	if parentIDStr != "" {
		pid, err := uuid.Parse(parentIDStr)
		if err != nil {
			h.renderScopes(w, r, "invalid parent scope id")
			return
		}
		parentID = &pid
	}
	if h.pool == nil {
		h.renderScopes(w, r, "service unavailable")
		return
	}
	if parentID != nil && !h.hasScopeAdminAccess(r.Context(), r, *parentID) {
		h.renderScopes(w, r, "scope admin required")
		return
	}
	if _, err := db.CreateScope(r.Context(), h.pool, kind, externalID, name, parentID, principalID, nil); err != nil {
		h.renderScopes(w, r, err.Error())
		return
	}
	http.Redirect(w, r, "/ui/scopes", http.StatusSeeOther)
}

// handleSetScopeRepo serves POST /ui/scopes/{id}/repo.
func (h *Handler) handleSetScopeRepo(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimPrefix(r.URL.Path, "/ui/scopes/")
	idStr := strings.TrimSuffix(trimmed, "/repo")
	id, err := uuid.Parse(idStr)
	if err != nil {
		h.renderScopes(w, r, "invalid scope id")
		return
	}
	if err := r.ParseForm(); err != nil {
		h.renderScopes(w, r, "bad form data")
		return
	}
	repoURL := r.FormValue("repo_url")
	defaultBranch := r.FormValue("default_branch")
	if repoURL == "" {
		h.renderScopes(w, r, "repo_url is required")
		return
	}
	if defaultBranch == "" {
		defaultBranch = "main"
	}
	if h.pool == nil {
		h.renderScopes(w, r, "service unavailable")
		return
	}
	if !h.hasScopeAdminAccess(r.Context(), r, id) {
		h.renderScopes(w, r, "scope admin required")
		return
	}
	if _, err := db.SetScopeRepo(r.Context(), h.pool, id, repoURL, defaultBranch); err != nil {
		h.renderScopes(w, r, err.Error())
		return
	}
	http.Redirect(w, r, "/ui/scopes", http.StatusSeeOther)
}

// handleSyncScopeRepo serves POST /ui/scopes/{id}/repo/sync.
// Starts a background sync and redirects immediately.
func (h *Handler) handleSyncScopeRepo(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimPrefix(r.URL.Path, "/ui/scopes/")
	idStr := strings.TrimSuffix(trimmed, "/repo/sync")
	id, err := uuid.Parse(idStr)
	if err != nil {
		h.renderScopes(w, r, "invalid scope id")
		return
	}
	if h.pool == nil {
		h.renderScopes(w, r, "service unavailable")
		return
	}
	if !h.hasScopeAdminAccess(r.Context(), r, id) {
		h.renderScopes(w, r, "scope admin required")
		return
	}
	scope, err := db.GetScopeByID(r.Context(), h.pool, id)
	if err != nil || scope == nil {
		h.renderScopes(w, r, "scope not found")
		return
	}
	if scope.RepoUrl == nil || *scope.RepoUrl == "" {
		h.renderScopes(w, r, "no repository attached to this scope")
		return
	}
	_ = r.ParseForm()
	prevCommit := ""
	if scope.LastIndexedCommit != nil {
		prevCommit = *scope.LastIndexedCommit
	}
	principalID, _ := r.Context().Value(auth.ContextKeyPrincipalID).(uuid.UUID)
	opts := codegraph.IndexOptions{
		ScopeID:          scope.ID,
		AuthorID:         principalID,
		RepoURL:          *scope.RepoUrl,
		DefaultBranch:    scope.RepoDefaultBranch,
		AuthToken:        r.FormValue("auth_token"),
		SSHKey:           r.FormValue("ssh_key"),
		SSHKeyPassphrase: r.FormValue("ssh_key_passphrase"),
		PrevCommit:       prevCommit,
	}
	h.syncer.Start(h.pool, opts) // fire and forget; status polled by UI
	http.Redirect(w, r, "/ui/scopes", http.StatusSeeOther)
}

// handleSyncStatus serves GET /ui/scopes/{id}/repo/sync/status.
// Returns JSON sync status for JS polling.
func (h *Handler) handleSyncStatus(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimPrefix(r.URL.Path, "/ui/scopes/")
	idStr := strings.TrimSuffix(trimmed, "/repo/sync/status")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "invalid scope id", http.StatusBadRequest)
		return
	}
	status := h.syncer.Status(id)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(status)
}

// handleAddMembership serves POST /ui/memberships.
func (h *Handler) handleAddMembership(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderPrincipals(w, r, "", "", "bad form data")
		return
	}
	memberIDStr := r.FormValue("member_id")
	parentIDStr := r.FormValue("parent_id")
	role := r.FormValue("role")
	if memberIDStr == "" || parentIDStr == "" || role == "" {
		h.renderPrincipals(w, r, "", "", "member, parent and role are required")
		return
	}
	memberID, err := uuid.Parse(memberIDStr)
	if err != nil {
		h.renderPrincipals(w, r, "", "", "invalid member id")
		return
	}
	parentID, err := uuid.Parse(parentIDStr)
	if err != nil {
		h.renderPrincipals(w, r, "", "", "invalid parent id")
		return
	}
	if h.pool == nil {
		h.renderPrincipals(w, r, "", "", "service unavailable")
		return
	}
	if !h.hasPrincipalAdminAccess(r.Context(), r, parentID) {
		h.renderPrincipals(w, r, "", "", "principal admin required")
		return
	}
	grantedBy := h.principalFromCookie(r)
	var grantedByPtr *uuid.UUID
	if grantedBy != uuid.Nil {
		grantedByPtr = &grantedBy
	}
	ms := principals.NewMembershipStore(h.pool)
	if err := ms.AddMembership(r.Context(), memberID, parentID, role, grantedByPtr); err != nil {
		h.renderPrincipals(w, r, "", "", err.Error())
		return
	}
	http.Redirect(w, r, "/ui/principals", http.StatusSeeOther)
}

// handleDeleteMembership serves POST /ui/memberships/delete.
func (h *Handler) handleDeleteMembership(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderPrincipals(w, r, "", "", "bad form data")
		return
	}
	memberIDStr := r.FormValue("member_id")
	parentIDStr := r.FormValue("parent_id")
	memberID, err := uuid.Parse(memberIDStr)
	if err != nil {
		h.renderPrincipals(w, r, "", "", "invalid member id")
		return
	}
	parentID, err := uuid.Parse(parentIDStr)
	if err != nil {
		h.renderPrincipals(w, r, "", "", "invalid parent id")
		return
	}
	if h.pool == nil {
		h.renderPrincipals(w, r, "", "", "service unavailable")
		return
	}
	if !h.hasPrincipalAdminAccess(r.Context(), r, parentID) {
		h.renderPrincipals(w, r, "", "", "principal admin required")
		return
	}
	if err := db.DeleteMembership(r.Context(), h.pool, memberID, parentID); err != nil {
		h.renderPrincipals(w, r, "", "", err.Error())
		return
	}
	http.Redirect(w, r, "/ui/principals", http.StatusSeeOther)
}

// principalFromCookie resolves the principal ID from the pb_session cookie.
// Returns uuid.Nil if the cookie is missing or invalid.
func (h *Handler) principalFromCookie(r *http.Request) uuid.UUID {
	token := h.tokenFromCookie(r)
	if token == nil {
		return uuid.Nil
	}
	return token.PrincipalID
}

// tokenFromCookie resolves the current session token from the pb_session cookie.
// Returns nil if the cookie is missing or invalid.
func (h *Handler) tokenFromCookie(r *http.Request) *db.Token {
	cookie, err := r.Cookie(cookieName)
	if err != nil || cookie.Value == "" || h.pool == nil {
		return nil
	}
	hash := auth.HashToken(cookie.Value)
	token, err := auth.NewTokenStore(h.pool).Lookup(r.Context(), hash)
	if err != nil || token == nil {
		return nil
	}
	return token
}

func (h *Handler) hasScopeAdminAccess(ctx context.Context, r *http.Request, scopeID uuid.UUID) bool {
	if h.pool == nil {
		return false
	}
	token := h.tokenFromCookie(r)
	if token == nil || token.PrincipalID == uuid.Nil {
		return false
	}
	if token.ScopeIds != nil {
		allowed := false
		for _, id := range token.ScopeIds {
			if id == scopeID {
				allowed = true
				break
			}
		}
		if !allowed {
			return false
		}
	}
	ms := principals.NewMembershipStore(h.pool)
	ok, err := ms.IsScopeAdmin(ctx, token.PrincipalID, scopeID)
	return err == nil && ok
}

func (h *Handler) hasPrincipalAdminAccess(ctx context.Context, r *http.Request, targetPrincipalID uuid.UUID) bool {
	if h.pool == nil {
		return false
	}
	token := h.tokenFromCookie(r)
	if token == nil || token.PrincipalID == uuid.Nil {
		return false
	}
	ms := principals.NewMembershipStore(h.pool)
	ok, err := ms.IsPrincipalAdmin(ctx, token.PrincipalID, targetPrincipalID)
	return err == nil && ok
}

func (h *Handler) hasAnyPrincipalAdminRole(ctx context.Context, r *http.Request) bool {
	if h.pool == nil {
		return false
	}
	token := h.tokenFromCookie(r)
	if token == nil || token.PrincipalID == uuid.Nil {
		return false
	}
	ms := principals.NewMembershipStore(h.pool)
	ok, err := ms.HasAnyAdminRole(ctx, token.PrincipalID)
	return err == nil && ok
}

// authorizedScopesForRequest resolves scopes writable by the current principal,
// intersected with token scope restrictions (when scope_ids is non-nil).
// Token scope restrictions are expanded to include ancestor scopes so a token
// scoped to a child scope can still access its parent scopes.
func (h *Handler) authorizedScopesForRequest(ctx context.Context, r *http.Request) ([]*db.Scope, map[uuid.UUID]struct{}) {
	out := map[uuid.UUID]struct{}{}
	if h.pool == nil {
		return []*db.Scope{}, out
	}
	token := h.tokenFromCookie(r)
	if token == nil || token.PrincipalID == uuid.Nil {
		return []*db.Scope{}, out
	}
	ms := principals.NewMembershipStore(h.pool)
	ids, err := ms.EffectiveScopeIDs(ctx, token.PrincipalID)
	if err != nil {
		return []*db.Scope{}, out
	}
	if token.ScopeIds != nil {
		allowed := make(map[uuid.UUID]struct{}, len(token.ScopeIds))
		for _, id := range token.ScopeIds {
			allowed[id] = struct{}{}
			ancestorIDs, err := db.GetAncestorScopeIDs(ctx, h.pool, id)
			if err == nil {
				for _, ancestorID := range ancestorIDs {
					allowed[ancestorID] = struct{}{}
				}
			}
		}
		intersected := make([]uuid.UUID, 0, len(ids))
		for _, id := range ids {
			if _, ok := allowed[id]; ok {
				intersected = append(intersected, id)
			}
		}
		// If no effective scopes are resolved for the principal, fall back to
		// explicit token scope restrictions so scoped session tokens still work.
		if len(ids) == 0 {
			for id := range allowed {
				intersected = append(intersected, id)
			}
		}
		ids = intersected
	}
	scopes, err := db.GetScopesByIDs(ctx, h.pool, ids)
	if err != nil {
		return []*db.Scope{}, out
	}
	for _, s := range scopes {
		out[s.ID] = struct{}{}
	}
	return scopes, out
}

// effectivePrincipalScopesForRequest resolves the current principal's effective scopes
// via ownership/memberships, without applying current token scope restrictions.
func (h *Handler) effectivePrincipalScopesForRequest(ctx context.Context, r *http.Request) ([]*db.Scope, map[uuid.UUID]struct{}) {
	out := map[uuid.UUID]struct{}{}
	if h.pool == nil {
		return []*db.Scope{}, out
	}
	token := h.tokenFromCookie(r)
	if token == nil || token.PrincipalID == uuid.Nil {
		return []*db.Scope{}, out
	}
	ms := principals.NewMembershipStore(h.pool)
	ids, err := ms.EffectiveScopeIDs(ctx, token.PrincipalID)
	if err != nil {
		return []*db.Scope{}, out
	}
	scopes, err := db.GetScopesByIDs(ctx, h.pool, ids)
	if err != nil {
		return []*db.Scope{}, out
	}
	for _, s := range scopes {
		out[s.ID] = struct{}{}
	}
	return scopes, out
}

func (h *Handler) reachablePrincipalIDSet(ctx context.Context, r *http.Request) map[uuid.UUID]struct{} {
	out := map[uuid.UUID]struct{}{}
	if h.pool == nil {
		return out
	}
	principalID := h.principalFromCookie(r)
	if principalID == uuid.Nil {
		return out
	}
	ids, err := db.GetAllParentIDs(ctx, h.pool, principalID)
	if err != nil {
		return out
	}
	for _, id := range ids {
		out[id] = struct{}{}
	}
	return out
}

// handleEndorseArtifact serves POST /ui/knowledge/{id}/endorse.
func (h *Handler) handleEndorseArtifact(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/ui/knowledge/"), "/endorse")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "invalid artifact id", http.StatusBadRequest)
		return
	}
	endorserID := h.principalFromCookie(r)
	if _, err := h.knwLife.Endorse(r.Context(), id, endorserID, nil); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, r.Referer(), http.StatusSeeOther)
}

// handleKnowledgeReview serves POST /ui/knowledge/{id}/review.
func (h *Handler) handleKnowledgeReview(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/ui/knowledge/"), "/review")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "invalid artifact id", http.StatusBadRequest)
		return
	}
	callerID := h.principalFromCookie(r)
	if err := h.knwLife.SubmitForReview(r.Context(), id, callerID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/ui/knowledge/"+id.String(), http.StatusSeeOther)
}

// handleKnowledgeDeprecate serves POST /ui/knowledge/{id}/deprecate.
func (h *Handler) handleKnowledgeDeprecate(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/ui/knowledge/"), "/deprecate")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "invalid artifact id", http.StatusBadRequest)
		return
	}
	callerID := h.principalFromCookie(r)
	if err := h.knwLife.Deprecate(r.Context(), id, callerID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/ui/knowledge/"+id.String(), http.StatusSeeOther)
}

// handleKnowledgeRepublish serves POST /ui/knowledge/{id}/republish.
func (h *Handler) handleKnowledgeRepublish(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/ui/knowledge/"), "/republish")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "invalid artifact id", http.StatusBadRequest)
		return
	}
	callerID := h.principalFromCookie(r)
	if err := h.knwLife.Republish(r.Context(), id, callerID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/ui/knowledge/"+id.String(), http.StatusSeeOther)
}

// handleKnowledgeDelete serves POST /ui/knowledge/{id}/delete.
func (h *Handler) handleKnowledgeDelete(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/ui/knowledge/"), "/delete")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "invalid artifact id", http.StatusBadRequest)
		return
	}
	callerID := h.principalFromCookie(r)
	if err := h.knwLife.Delete(r.Context(), id, callerID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/ui/knowledge", http.StatusSeeOther)
}

// handleKnowledgeRetract serves POST /ui/knowledge/{id}/retract.
func (h *Handler) handleKnowledgeRetract(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/ui/knowledge/"), "/retract")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "invalid artifact id", http.StatusBadRequest)
		return
	}
	if h.knwLife == nil {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
		return
	}
	callerID := h.principalFromCookie(r)
	if err := h.knwLife.RetractToDraft(r.Context(), id, callerID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/ui/knowledge/"+id.String(), http.StatusSeeOther)
}

// handleKnowledgeNew serves GET /ui/knowledge/new.
func (h *Handler) handleKnowledgeNew(w http.ResponseWriter, r *http.Request) {
	h.renderKnowledgeNew(w, r, "")
}

func (h *Handler) renderKnowledgeNew(w http.ResponseWriter, r *http.Request, formError string) {
	data := struct {
		FormError string
		Scopes    []*db.Scope
	}{FormError: formError}
	if h.pool != nil {
		scopes, _ := h.authorizedScopesForRequest(r.Context(), r)
		data.Scopes = scopes
	}
	h.render(w, r, "knowledge_new", "New Knowledge Article", data)
}

// handleCreateKnowledge serves POST /ui/knowledge.
func (h *Handler) handleCreateKnowledge(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" {
		h.renderKnowledgeNew(w, r, "title is required")
		return
	}
	scopeStr := strings.TrimSpace(r.FormValue("scope_id"))
	if scopeStr == "" {
		h.renderKnowledgeNew(w, r, "scope is required")
		return
	}
	scopeID, err := uuid.Parse(scopeStr)
	if err != nil {
		h.renderKnowledgeNew(w, r, "invalid scope id")
		return
	}
	_, authorizedScopeSet := h.authorizedScopesForRequest(r.Context(), r)
	if _, ok := authorizedScopeSet[scopeID]; !ok {
		h.renderKnowledgeNew(w, r, "scope access denied")
		return
	}
	if h.knwStore == nil {
		h.renderKnowledgeNew(w, r, "service unavailable")
		return
	}
	content := r.FormValue("content")
	visibility := r.FormValue("visibility")
	if visibility == "" {
		visibility = "team"
	}
	authorID := h.principalFromCookie(r)
	art, err := h.knwStore.Create(r.Context(), knowledge.CreateInput{
		KnowledgeType: "semantic",
		OwnerScopeID:  scopeID,
		AuthorID:      authorID,
		Visibility:    visibility,
		Title:         title,
		Content:       content,
	})
	if err != nil {
		h.renderKnowledgeNew(w, r, err.Error())
		return
	}
	http.Redirect(w, r, "/ui/knowledge/"+art.ID.String(), http.StatusSeeOther)
}

// handleMemoryForget serves POST /ui/memories/{id}/forget.
func (h *Handler) handleMemoryForget(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimPrefix(r.URL.Path, "/ui/memories/")
	idStr := strings.TrimSuffix(trimmed, "/forget")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "invalid memory id", http.StatusBadRequest)
		return
	}
	if h.pool == nil {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
		return
	}
	if err := db.SoftDeleteMemory(r.Context(), h.pool, id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/ui/memories", http.StatusSeeOther)
}

// handleCollectionNew serves GET /ui/collections/new.
func (h *Handler) handleCollectionNew(w http.ResponseWriter, r *http.Request) {
	h.renderCollectionNew(w, r, "")
}

func (h *Handler) renderCollectionNew(w http.ResponseWriter, r *http.Request, formError string) {
	data := struct {
		FormError string
		Scopes    []*db.Scope
	}{FormError: formError}
	if h.pool != nil {
		scopes, _ := h.authorizedScopesForRequest(r.Context(), r)
		data.Scopes = scopes
	}
	h.render(w, r, "collections_new", "New Collection", data)
}

// handleCreateCollection serves POST /ui/collections.
func (h *Handler) handleCreateCollection(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		h.renderCollectionNew(w, r, "name is required")
		return
	}
	slug := strings.TrimSpace(r.FormValue("slug"))
	if slug == "" {
		h.renderCollectionNew(w, r, "slug is required")
		return
	}
	scopeStr := strings.TrimSpace(r.FormValue("scope_id"))
	if scopeStr == "" {
		h.renderCollectionNew(w, r, "scope is required")
		return
	}
	scopeID, err := uuid.Parse(scopeStr)
	if err != nil {
		h.renderCollectionNew(w, r, "invalid scope id")
		return
	}
	_, authorizedScopeSet := h.authorizedScopesForRequest(r.Context(), r)
	if _, ok := authorizedScopeSet[scopeID]; !ok {
		h.renderCollectionNew(w, r, "scope access denied")
		return
	}
	if h.pool == nil {
		h.renderCollectionNew(w, r, "service unavailable")
		return
	}
	visibility := r.FormValue("visibility")
	if visibility == "" {
		visibility = "team"
	}
	ownerID := h.principalFromCookie(r)
	coll, err := db.CreateCollection(r.Context(), h.pool, &db.KnowledgeCollection{
		ScopeID:    scopeID,
		OwnerID:    ownerID,
		Name:       name,
		Slug:       slug,
		Visibility: visibility,
	})
	if err != nil {
		h.renderCollectionNew(w, r, err.Error())
		return
	}
	http.Redirect(w, r, "/ui/collections/"+coll.ID.String(), http.StatusSeeOther)
}

// handleQuery serves GET /ui/query — the recall playground.
func (h *Handler) handleQuery(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	scopes, scopeSet := h.authorizedScopesForRequest(ctx, r)

	type queryResult struct {
		Layer         string
		ID            string
		Score         float64
		Title         string
		Content       string
		MemoryType    string
		KnowledgeType string
		SourceRef     string
		Status        string
		Visibility    string
		Endorsements  int
	}

	data := struct {
		Title      string
		Content    template.HTML
		Scopes     []*db.Scope
		Query      string
		ScopeID    string
		Layers     map[string]bool
		SearchMode string
		Limit      int
		Results    []queryResult
		Ran        bool
		Error      string
	}{
		Title:      "Query Playground",
		Scopes:     scopes,
		Layers:     map[string]bool{"memory": true, "knowledge": true, "skill": true},
		SearchMode: "hybrid",
		Limit:      10,
	}

	if r.Method == http.MethodGet && r.URL.Query().Get("q") != "" {
		data.Query = r.URL.Query().Get("q")
		data.ScopeID = r.URL.Query().Get("scope_id")
		data.SearchMode = r.URL.Query().Get("search_mode")
		if data.SearchMode == "" {
			data.SearchMode = "hybrid"
		}
		if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 {
			data.Limit = l
		}
		// Layers from checkboxes.
		data.Layers = map[string]bool{
			"memory":    r.URL.Query().Get("layer_memory") == "1",
			"knowledge": r.URL.Query().Get("layer_knowledge") == "1",
			"skill":     r.URL.Query().Get("layer_skill") == "1",
		}
		// Default: all layers if none checked.
		if !data.Layers["memory"] && !data.Layers["knowledge"] && !data.Layers["skill"] {
			data.Layers = map[string]bool{"memory": true, "knowledge": true, "skill": true}
		}

		scopeID, err := uuid.Parse(data.ScopeID)
		if err != nil && len(scopes) > 0 {
			scopeID = scopes[0].ID
			data.ScopeID = scopeID.String()
		}
		if err == nil {
			if _, ok := scopeSet[scopeID]; !ok && len(scopes) > 0 {
				scopeID = scopes[0].ID
				data.ScopeID = scopeID.String()
			}
		}

		principalID := h.principalFromCookie(r)
		authorizedScopeIDs := make([]uuid.UUID, 0, len(scopeSet))
		for id := range scopeSet {
			authorizedScopeIDs = append(authorizedScopeIDs, id)
		}

		activeLayers := map[retrieval.Layer]bool{
			retrieval.LayerMemory:    data.Layers["memory"],
			retrieval.LayerKnowledge: data.Layers["knowledge"],
			retrieval.LayerSkill:     data.Layers["skill"],
		}

		merged, err := retrieval.OrchestrateRecall(ctx, retrieval.OrchestrateDeps{
			Pool:     h.pool,
			MemStore: h.memStore,
			KnwStore: h.knwStore,
			Svc:      h.svc,
		}, retrieval.OrchestrateInput{
			Query:              data.Query,
			ScopeID:            scopeID,
			PrincipalID:        principalID,
			AuthorizedScopeIDs: authorizedScopeIDs,
			SearchMode:         data.SearchMode,
			Limit:              data.Limit,
			MinScore:           0,
			GraphDepth:         1,
			ActiveLayers:       activeLayers,
		})
		if err != nil {
			data.Error = "query recall: " + err.Error()
		}
		for _, res := range merged {
			content := res.Content
			if res.Layer == retrieval.LayerSkill && content == "" {
				content = res.Description
			}
			title := res.Title
			if res.Layer == retrieval.LayerSkill {
				title = res.Name
			}
			data.Results = append(data.Results, queryResult{
				Layer:         string(res.Layer),
				ID:            res.ID.String(),
				Score:         res.Score,
				Title:         title,
				Content:       content,
				MemoryType:    res.MemoryType,
				KnowledgeType: res.KnowledgeType,
				SourceRef:     res.SourceRef,
				Status:        res.Status,
				Visibility:    res.Visibility,
				Endorsements:  res.Endorsements,
			})
		}
		data.Ran = true
	} else if len(scopes) > 0 {
		data.ScopeID = scopes[0].ID.String()
	}

	h.render(w, r, "query", "Query Playground", data)
}

// handleMetrics serves GET /ui/metrics.
func (h *Handler) handleMetrics(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, "metrics", "Metrics", nil)
}

// handleUploadKnowledge serves POST /ui/knowledge/upload.
func (h *Handler) handleUploadKnowledge(w http.ResponseWriter, r *http.Request) {
	if h.knwStore == nil {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "file too large or invalid form", http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file field is required", http.StatusBadRequest)
		return
	}
	defer closeutil.Log(file, "ui knowledge upload multipart file")

	data, err := io.ReadAll(io.LimitReader(file, 32<<20))
	if err != nil {
		http.Error(w, "failed to read file", http.StatusInternalServerError)
		return
	}

	text, err := ingest.Extract(header.Filename, data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(text) == "" {
		http.Error(w, "extracted text is empty", http.StatusBadRequest)
		return
	}

	scopeStr := r.FormValue("scope")
	if scopeStr == "" {
		http.Error(w, "scope is required", http.StatusBadRequest)
		return
	}
	parts := strings.SplitN(scopeStr, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		http.Error(w, "scope must be kind:external_id", http.StatusBadRequest)
		return
	}
	scope, err := db.GetScopeByExternalID(r.Context(), h.pool, parts[0], parts[1])
	if err != nil || scope == nil {
		http.Error(w, "scope not found", http.StatusBadRequest)
		return
	}

	title := r.FormValue("title")
	if title == "" {
		base := filepath.Base(header.Filename)
		title = strings.TrimSuffix(base, filepath.Ext(base))
	}

	knowledgeType := r.FormValue("knowledge_type")
	if knowledgeType == "" {
		knowledgeType = "reference"
	}

	authorID := h.principalFromCookie(r)

	workflow := r.FormValue("workflow")
	_, err = h.knwStore.Create(r.Context(), knowledge.CreateInput{
		KnowledgeType: knowledgeType,
		OwnerScopeID:  scope.ID,
		AuthorID:      authorID,
		Visibility:    "team",
		Title:         title,
		Content:       text,
		AutoReview:    workflow == "review",
		AutoPublish:   workflow == "publish",
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/ui/knowledge", http.StatusSeeOther)
}
