// Package ui provides the HTTP handler for the Postbrain Web UI.
package ui

import (
	"embed"
	"html/template"
	"io"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/embedding"
	"github.com/simplyblock/postbrain/internal/ingest"
	"github.com/simplyblock/postbrain/internal/knowledge"
	"github.com/simplyblock/postbrain/internal/principals"
)

//go:embed web/templates
var templatesFS embed.FS

//go:embed web/static
var staticFS embed.FS

// Handler holds all dependencies for the UI.
type Handler struct {
	pool      *pgxpool.Pool
	templates *template.Template
	staticFS  fs.FS
	knwLife   *knowledge.Lifecycle
	knwStore  *knowledge.Store
	knwProm   *knowledge.Promoter
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
	}
	tmpl, err := template.New("").Funcs(funcMap).ParseFS(templatesFS, "web/templates/*.html")
	if err != nil {
		return nil, err
	}
	sub, err := fs.Sub(staticFS, "web/static")
	if err != nil {
		return nil, err
	}
	h := &Handler{pool: pool, templates: tmpl, staticFS: sub}
	if pool != nil {
		membership := principals.NewMembershipStore(pool)
		h.knwLife = knowledge.NewLifecycle(pool, membership)
		h.knwProm = knowledge.NewPromoter(pool, svc)
	}
	if pool != nil && svc != nil {
		h.knwStore = knowledge.NewStore(pool, svc)
	}
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
	// Auth check for all other routes.
	if !h.authenticated(w, r) {
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
	case r.URL.Path == "/ui/knowledge":
		h.handleKnowledge(w, r)
	case strings.HasSuffix(r.URL.Path, "/endorse") && strings.HasPrefix(r.URL.Path, "/ui/knowledge/") && r.Method == http.MethodPost:
		h.handleEndorseArtifact(w, r)
	case strings.HasSuffix(r.URL.Path, "/history") && strings.HasPrefix(r.URL.Path, "/ui/knowledge/"):
		h.handleKnowledgeHistory(w, r)
	case strings.HasPrefix(r.URL.Path, "/ui/knowledge/"):
		h.handleKnowledgeDetail(w, r)
	case r.URL.Path == "/ui/collections":
		h.handleCollections(w, r)
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
	case r.URL.Path == "/ui/skills":
		h.handleSkills(w, r)
	case strings.HasSuffix(r.URL.Path, "/history") && strings.HasPrefix(r.URL.Path, "/ui/skills/"):
		h.handleSkillHistory(w, r)
	case strings.HasPrefix(r.URL.Path, "/ui/skills/"):
		h.handleSkillDetail(w, r)
	case r.URL.Path == "/ui/principals" && r.Method == http.MethodPost:
		h.handleCreatePrincipal(w, r)
	case r.URL.Path == "/ui/principals":
		h.handlePrincipals(w, r)
	case r.URL.Path == "/ui/scopes" && r.Method == http.MethodPost:
		h.handleCreateScope(w, r)
	case strings.HasSuffix(r.URL.Path, "/delete") && strings.HasPrefix(r.URL.Path, "/ui/scopes/") && r.Method == http.MethodPost:
		h.handleDeleteScope(w, r)
	case r.URL.Path == "/ui/memberships" && r.Method == http.MethodPost:
		h.handleAddMembership(w, r)
	case r.URL.Path == "/ui/memberships/delete" && r.Method == http.MethodPost:
		h.handleDeleteMembership(w, r)
	case r.URL.Path == "/ui/metrics":
		h.handleMetrics(w, r)
	default:
		http.NotFound(w, r)
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
	if err := h.templates.ExecuteTemplate(w, "base", struct {
		Title   string
		Content template.HTML
	}{
		Title:   title,
		Content: template.HTML(buf.String()), //nolint:gosec // generated by our own templates
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
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

// handleMemories serves GET /ui/memories.
func (h *Handler) handleMemories(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	scopeID := r.URL.Query().Get("scope_id")

	data := struct {
		Query      string
		ScopeID    string
		Memories   []*db.Memory
		NextCursor string
	}{
		Query:    q,
		ScopeID:  scopeID,
		Memories: nil,
	}

	if h.pool != nil && scopeID != "" {
		sid, err := uuid.Parse(scopeID)
		if err == nil {
			mems, err := db.ListMemoriesByScope(r.Context(), h.pool, sid, 20, 0)
			if err == nil {
				data.Memories = mems
			}
		}
	}

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

	data := struct {
		Memory *db.Memory
	}{}

	if h.pool != nil {
		mem, err := db.GetMemory(r.Context(), h.pool, id)
		if err != nil || mem == nil {
			http.NotFound(w, r)
			return
		}
		data.Memory = mem
	}

	h.render(w, r, "memory_detail", "Memory", data)
}

// handleKnowledge serves GET /ui/knowledge.
func (h *Handler) handleKnowledge(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	_ = q

	data := struct {
		Query       string
		Artifacts   []*db.KnowledgeArtifact
		UploadError string
	}{
		Query:     q,
		Artifacts: nil,
	}

	if h.pool != nil {
		arts, err := db.ListAllArtifacts(r.Context(), h.pool, 20, 0)
		if err == nil {
			data.Artifacts = arts
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

	data := struct {
		Artifact *db.KnowledgeArtifact
	}{}

	if h.pool != nil {
		art, err := db.GetArtifact(r.Context(), h.pool, id)
		if err != nil || art == nil {
			http.NotFound(w, r)
			return
		}
		data.Artifact = art
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
		var colls []*db.KnowledgeCollection
		scopeStr := r.URL.Query().Get("scope_id")
		if scopeStr != "" {
			sid, err := uuid.Parse(scopeStr)
			if err == nil {
				colls, _ = db.ListCollections(r.Context(), h.pool, sid)
			}
		} else {
			colls, _ = db.ListAllCollections(r.Context(), h.pool)
		}
		data.Collections = colls
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

	data := struct {
		Collection *db.KnowledgeCollection
		Artifacts  []*db.KnowledgeArtifact
	}{}

	if h.pool != nil {
		coll, err := db.GetCollection(r.Context(), h.pool, id)
		if err != nil || coll == nil {
			http.NotFound(w, r)
			return
		}
		data.Collection = coll
		arts, _ := db.ListCollectionItems(r.Context(), h.pool, id)
		data.Artifacts = arts
	}

	h.render(w, r, "collection_detail", "Collection", data)
}

// handlePromotions serves GET /ui/promotions.
func (h *Handler) handlePromotions(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Promotions []*db.PromotionRequest
	}{}

	if h.pool != nil {
		proms, err := db.ListPendingPromotions(r.Context(), h.pool, uuid.Nil)
		if err == nil {
			data.Promotions = proms
		}
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
		flags, err := db.ListStalenessFlags(r.Context(), h.pool, "open", 50, 0)
		if err == nil {
			data.Flags = flags
		}
	}

	h.render(w, r, "staleness", "Staleness Flags", data)
}

// handleGraph serves GET /ui/graph.
func (h *Handler) handleGraph(w http.ResponseWriter, r *http.Request) {
	scopeStr := r.URL.Query().Get("scope_id")

	data := struct {
		ScopeID        string
		Entities       []*db.Entity
		Relations      []*db.Relation
		EntityNames    map[uuid.UUID]string
		RelationCounts map[uuid.UUID]int
	}{
		ScopeID:        scopeStr,
		EntityNames:    map[uuid.UUID]string{},
		RelationCounts: map[uuid.UUID]int{},
	}

	if h.pool != nil && scopeStr != "" {
		sid, err := uuid.Parse(scopeStr)
		if err == nil {
			ents, err := db.ListEntitiesByScope(r.Context(), h.pool, sid, "", 100, 0)
			if err == nil {
				data.Entities = ents
				for _, e := range ents {
					data.EntityNames[e.ID] = e.Name
				}
			}
			rels, err := db.ListRelationsByScope(r.Context(), h.pool, sid, 200, 0)
			if err == nil {
				data.Relations = rels
				for _, rel := range rels {
					data.RelationCounts[rel.SubjectID]++
					data.RelationCounts[rel.ObjectID]++
				}
			}
		}
	}

	h.render(w, r, "graph", "Entity Graph", data)
}

// handleSkills serves GET /ui/skills.
func (h *Handler) handleSkills(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Skills []*db.Skill
	}{}

	if h.pool != nil {
		skills, err := db.ListPublishedSkillsForAgent(r.Context(), h.pool, nil, "")
		if err == nil {
			data.Skills = skills
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

// renderPrincipals renders the principals+scopes page with optional form errors.
func (h *Handler) renderPrincipals(w http.ResponseWriter, r *http.Request, principalErr, scopeErr, membershipErr string) {
	data := struct {
		Principals          []*db.Principal
		Scopes              []*db.Scope
		Memberships         []*db.MembershipRow
		PrincipalFormError  string
		ScopeFormError      string
		MembershipFormError string
	}{PrincipalFormError: principalErr, ScopeFormError: scopeErr, MembershipFormError: membershipErr}

	if h.pool != nil {
		principals, err := db.ListPrincipals(r.Context(), h.pool, 50, 0)
		if err == nil {
			data.Principals = principals
		}
		scopes, err := db.ListScopes(r.Context(), h.pool, 50, 0)
		if err == nil {
			data.Scopes = scopes
		}
		memberships, err := db.ListAllMemberships(r.Context(), h.pool)
		if err == nil {
			data.Memberships = memberships
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
		h.renderPrincipals(w, r, "", "invalid scope id", "")
		return
	}
	if h.pool == nil {
		h.renderPrincipals(w, r, "", "service unavailable", "")
		return
	}
	if err := db.DeleteScope(r.Context(), h.pool, id); err != nil {
		h.renderPrincipals(w, r, "", err.Error(), "")
		return
	}
	http.Redirect(w, r, "/ui/principals", http.StatusSeeOther)
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
	ps := principals.NewStore(h.pool)
	if _, err := ps.Create(r.Context(), kind, slug, displayName, nil); err != nil {
		h.renderPrincipals(w, r, err.Error(), "", "")
		return
	}
	http.Redirect(w, r, "/ui/principals", http.StatusSeeOther)
}

// handleCreateScope serves POST /ui/scopes.
func (h *Handler) handleCreateScope(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderPrincipals(w, r, "", "bad form data", "")
		return
	}
	kind := r.FormValue("kind")
	externalID := r.FormValue("external_id")
	name := r.FormValue("name")
	principalIDStr := r.FormValue("principal_id")
	parentIDStr := r.FormValue("parent_id")

	if kind == "" || externalID == "" || name == "" || principalIDStr == "" {
		h.renderPrincipals(w, r, "", "kind, external_id, name and principal are required", "")
		return
	}
	principalID, err := uuid.Parse(principalIDStr)
	if err != nil {
		h.renderPrincipals(w, r, "", "invalid principal id", "")
		return
	}
	var parentID *uuid.UUID
	if parentIDStr != "" {
		pid, err := uuid.Parse(parentIDStr)
		if err != nil {
			h.renderPrincipals(w, r, "", "invalid parent scope id", "")
			return
		}
		parentID = &pid
	}
	if h.pool == nil {
		h.renderPrincipals(w, r, "", "service unavailable", "")
		return
	}
	if _, err := db.CreateScope(r.Context(), h.pool, kind, externalID, name, parentID, principalID, nil); err != nil {
		h.renderPrincipals(w, r, "", err.Error(), "")
		return
	}
	http.Redirect(w, r, "/ui/principals", http.StatusSeeOther)
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
	if err := db.DeleteMembership(r.Context(), h.pool, memberID, parentID); err != nil {
		h.renderPrincipals(w, r, "", "", err.Error())
		return
	}
	http.Redirect(w, r, "/ui/principals", http.StatusSeeOther)
}

// principalFromCookie resolves the principal ID from the pb_session cookie.
// Returns uuid.Nil if the cookie is missing or invalid.
func (h *Handler) principalFromCookie(r *http.Request) uuid.UUID {
	cookie, err := r.Cookie(cookieName)
	if err != nil || cookie.Value == "" || h.pool == nil {
		return uuid.Nil
	}
	hash := auth.HashToken(cookie.Value)
	token, err := auth.NewTokenStore(h.pool).Lookup(r.Context(), hash)
	if err != nil || token == nil {
		return uuid.Nil
	}
	return token.PrincipalID
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
	http.Redirect(w, r, "/ui/knowledge", http.StatusSeeOther)
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
	defer file.Close()

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

	_, err = h.knwStore.Create(r.Context(), knowledge.CreateInput{
		KnowledgeType: knowledgeType,
		OwnerScopeID:  scope.ID,
		AuthorID:      authorID,
		Visibility:    "team",
		Title:         title,
		Content:       text,
		AutoReview:    r.FormValue("auto_review") == "on",
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/ui/knowledge", http.StatusSeeOther)
}
