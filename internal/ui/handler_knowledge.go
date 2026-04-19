package ui

import (
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/simplyblock/postbrain/internal/closeutil"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/db/compat"
	"github.com/simplyblock/postbrain/internal/ingest"
	"github.com/simplyblock/postbrain/internal/knowledge"
)

const knowledgePageSize = 50

// handleKnowledge serves GET /ui/{scope}/knowledge.
func (h *Handler) handleKnowledge(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	status := r.URL.Query().Get("status")

	cursor := 0
	if c, err := strconv.Atoi(r.URL.Query().Get("cursor")); err == nil && c > 0 {
		cursor = c
	}

	scope := scopeFromContext(r.Context())

	data := struct {
		Query         string
		Status        string
		ScopeID       string
		Artifacts     []*db.KnowledgeArtifact
		ArtifactKinds []string
		UploadError   string
		PrevCursor    int
		NextCursor    int
		HasPrev       bool
		HasNext       bool
	}{
		Query:         q,
		Status:        status,
		ArtifactKinds: knowledge.ArtifactKinds(),
		PrevCursor:    cursor - knowledgePageSize,
		HasPrev:       cursor > 0,
	}
	if scope != nil {
		data.ScopeID = scope.ID.String()
	}

	if h.pool != nil && scope != nil {
		_, scopeSet := h.authorizedScopesForRequest(r.Context(), r)
		var arts []*db.KnowledgeArtifact
		var err error
		if q != "" {
			arts, err = compat.SearchArtifacts(r.Context(), h.pool, q, status, scope.ID, knowledgePageSize+1, cursor)
		} else if status != "" {
			arts, err = compat.ListArtifactsByStatus(r.Context(), h.pool, status, scope.ID, knowledgePageSize+1, cursor)
		} else {
			arts, err = compat.ListAllArtifacts(r.Context(), h.pool, scope.ID, knowledgePageSize+1, cursor)
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

	tmpl := "knowledge"
	if r.Header.Get("HX-Request") == "true" {
		tmpl = "knowledge_rows"
	}
	h.render(w, r, tmpl, "Knowledge", data)
}

// handleKnowledgeDetail serves GET /ui/{scope}/knowledge/{id}.
func (h *Handler) handleKnowledgeDetail(w http.ResponseWriter, r *http.Request) {
	path := routePathFromContext(r.Context(), r)
	idStr := strings.TrimPrefix(path, "/ui/knowledge/")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if h.pool == nil {
		http.NotFound(w, r)
		return
	}

	scopeID := ""
	if scope := scopeFromContext(r.Context()); scope != nil {
		scopeID = scope.ID.String()
	}

	data := struct {
		Artifact *db.KnowledgeArtifact
		Sources  []*db.KnowledgeArtifact
		Digests  []*db.KnowledgeArtifact
		ScopeID  string
	}{ScopeID: scopeID}

	art, err := compat.GetArtifact(r.Context(), h.pool, id)
	if err != nil || art == nil {
		http.NotFound(w, r)
		return
	}
	// Verify the artifact belongs to the scope in the URL to prevent
	// cross-scope leakage. Return 404 to avoid revealing existence.
	if scope := scopeFromContext(r.Context()); scope == nil || art.OwnerScopeID != scope.ID {
		http.NotFound(w, r)
		return
	}
	data.Artifact = art

	if art.KnowledgeType == "digest" {
		sources, err := compat.ListDigestSources(r.Context(), h.pool, id)
		if err != nil {
			http.Error(w, "failed to load digest sources", http.StatusInternalServerError)
			return
		}
		data.Sources = sources
	} else {
		digests, err := compat.ListDigestsForSource(r.Context(), h.pool, id)
		if err != nil {
			http.Error(w, "failed to load digests", http.StatusInternalServerError)
			return
		}
		data.Digests = digests
	}

	h.render(w, r, "knowledge_detail", "Knowledge", data)
}

// handleKnowledgeHistory serves GET /ui/{scope}/knowledge/{id}/history.
func (h *Handler) handleKnowledgeHistory(w http.ResponseWriter, r *http.Request) {
	path := routePathFromContext(r.Context(), r)
	idStr := strings.TrimSuffix(strings.TrimPrefix(path, "/ui/knowledge/"), "/history")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	scopeID := ""
	if scope := scopeFromContext(r.Context()); scope != nil {
		scopeID = scope.ID.String()
	}

	data := struct {
		Artifact *db.KnowledgeArtifact
		History  []*db.KnowledgeHistory
		ScopeID  string
	}{ScopeID: scopeID}

	if h.pool != nil {
		art, err := compat.GetArtifact(r.Context(), h.pool, id)
		if err != nil || art == nil {
			http.NotFound(w, r)
			return
		}
		// Verify the artifact belongs to the scope in the URL to prevent
		// cross-scope leakage. Return 404 to avoid revealing existence.
		if scope := scopeFromContext(r.Context()); scope == nil || art.OwnerScopeID != scope.ID {
			http.NotFound(w, r)
			return
		}
		data.Artifact = art
		history, _ := compat.GetArtifactHistory(r.Context(), h.pool, id)
		data.History = history
	}

	h.render(w, r, "knowledge_history", "Knowledge History", data)
}

// handleEndorseArtifact serves POST /ui/{scope}/knowledge/{id}/endorse.
func (h *Handler) handleEndorseArtifact(w http.ResponseWriter, r *http.Request) {
	path := routePathFromContext(r.Context(), r)
	idStr := strings.TrimSuffix(strings.TrimPrefix(path, "/ui/knowledge/"), "/endorse")
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

// handleKnowledgeReview serves POST /ui/{scope}/knowledge/{id}/review.
func (h *Handler) handleKnowledgeReview(w http.ResponseWriter, r *http.Request) {
	path := routePathFromContext(r.Context(), r)
	idStr := strings.TrimSuffix(strings.TrimPrefix(path, "/ui/knowledge/"), "/review")
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
	http.Redirect(w, r, scopedPath(r, "/knowledge/"+id.String()), http.StatusSeeOther)
}

// handleKnowledgeDeprecate serves POST /ui/{scope}/knowledge/{id}/deprecate.
func (h *Handler) handleKnowledgeDeprecate(w http.ResponseWriter, r *http.Request) {
	path := routePathFromContext(r.Context(), r)
	idStr := strings.TrimSuffix(strings.TrimPrefix(path, "/ui/knowledge/"), "/deprecate")
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
	http.Redirect(w, r, scopedPath(r, "/knowledge/"+id.String()), http.StatusSeeOther)
}

// handleKnowledgeRepublish serves POST /ui/{scope}/knowledge/{id}/republish.
func (h *Handler) handleKnowledgeRepublish(w http.ResponseWriter, r *http.Request) {
	path := routePathFromContext(r.Context(), r)
	idStr := strings.TrimSuffix(strings.TrimPrefix(path, "/ui/knowledge/"), "/republish")
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
	http.Redirect(w, r, scopedPath(r, "/knowledge/"+id.String()), http.StatusSeeOther)
}

// handleKnowledgeDelete serves POST /ui/{scope}/knowledge/{id}/delete.
func (h *Handler) handleKnowledgeDelete(w http.ResponseWriter, r *http.Request) {
	path := routePathFromContext(r.Context(), r)
	idStr := strings.TrimSuffix(strings.TrimPrefix(path, "/ui/knowledge/"), "/delete")
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
	scopedRedirect(w, r, "/knowledge")
}

// handleKnowledgeRetract serves POST /ui/{scope}/knowledge/{id}/retract.
func (h *Handler) handleKnowledgeRetract(w http.ResponseWriter, r *http.Request) {
	path := routePathFromContext(r.Context(), r)
	idStr := strings.TrimSuffix(strings.TrimPrefix(path, "/ui/knowledge/"), "/retract")
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
	http.Redirect(w, r, scopedPath(r, "/knowledge/"+id.String()), http.StatusSeeOther)
}

// handleKnowledgeNew serves GET /ui/{scope}/knowledge/new.
func (h *Handler) handleKnowledgeNew(w http.ResponseWriter, r *http.Request) {
	h.renderKnowledgeNew(w, r, "")
}

func (h *Handler) renderKnowledgeNew(w http.ResponseWriter, r *http.Request, formError string) {
	scopeID := ""
	if scope := scopeFromContext(r.Context()); scope != nil {
		scopeID = scope.ID.String()
	}
	data := struct {
		FormError     string
		ScopeID       string
		ArtifactKinds []string
	}{
		FormError:     formError,
		ScopeID:       scopeID,
		ArtifactKinds: knowledge.ArtifactKinds(),
	}
	h.render(w, r, "knowledge_new", "New Knowledge Article", data)
}

// handleCreateKnowledge serves POST /ui/{scope}/knowledge.
func (h *Handler) handleCreateKnowledge(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	scope := scopeFromContext(r.Context())
	if scope == nil {
		h.renderKnowledgeNew(w, r, "scope not found")
		return
	}
	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" {
		h.renderKnowledgeNew(w, r, "title is required")
		return
	}
	artifactKind, err := knowledge.NormalizeArtifactKind(r.FormValue("artifact_kind"))
	if err != nil {
		h.renderKnowledgeNew(w, r, "invalid artifact kind")
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
		ArtifactKind:  artifactKind,
		OwnerScopeID:  scope.ID,
		AuthorID:      authorID,
		Visibility:    visibility,
		Title:         title,
		Content:       content,
	})
	if err != nil {
		h.renderKnowledgeNew(w, r, err.Error())
		return
	}
	http.Redirect(w, r, scopedPath(r, "/knowledge/"+art.ID.String()), http.StatusSeeOther)
}

// handleUploadKnowledge serves POST /ui/{scope}/knowledge/upload.
func (h *Handler) handleUploadKnowledge(w http.ResponseWriter, r *http.Request) {
	if h.knwStore == nil {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
		return
	}
	scope := scopeFromContext(r.Context())
	if scope == nil {
		http.Error(w, "scope not found", http.StatusBadRequest)
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

	title := r.FormValue("title")
	if title == "" {
		base := filepath.Base(header.Filename)
		title = strings.TrimSuffix(base, filepath.Ext(base))
	}

	knowledgeType := r.FormValue("knowledge_type")
	if knowledgeType == "" {
		knowledgeType = "reference"
	}
	artifactKind, err := knowledge.NormalizeArtifactKind(r.FormValue("artifact_kind"))
	if err != nil {
		http.Error(w, "invalid artifact kind", http.StatusBadRequest)
		return
	}

	authorID := h.principalFromCookie(r)

	workflow := r.FormValue("workflow")
	_, err = h.knwStore.Create(r.Context(), knowledge.CreateInput{
		KnowledgeType: knowledgeType,
		ArtifactKind:  artifactKind,
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

	scopedRedirect(w, r, "/knowledge")
}
