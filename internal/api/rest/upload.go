package rest

import (
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/closeutil"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/ingest"
	"github.com/simplyblock/postbrain/internal/knowledge"
)

const maxUploadSize = 32 << 20 // 32 MB

func (ro *Router) uploadKnowledge(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		writeError(w, http.StatusBadRequest, "file too large or invalid form")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file field is required")
		return
	}
	defer closeutil.Log(file, "knowledge upload multipart file")

	data, err := io.ReadAll(io.LimitReader(file, maxUploadSize))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read file")
		return
	}

	text, err := ingest.Extract(header.Filename, data)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(text) == "" {
		writeError(w, http.StatusBadRequest, "extracted text is empty")
		return
	}

	scopeStr := r.FormValue("scope")
	if scopeStr == "" {
		writeError(w, http.StatusBadRequest, "scope is required")
		return
	}
	kind, externalID, err := parseScopeString(scopeStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	scope, err := db.GetScopeByExternalID(r.Context(), ro.pool, kind, externalID)
	if err != nil || scope == nil {
		writeError(w, http.StatusBadRequest, "scope not found")
		return
	}
	if err := ro.authorizeRequestedScope(r.Context(), scope.ID); err != nil {
		writeScopeAuthzError(w, err)
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

	visibility := r.FormValue("visibility")
	if visibility == "" {
		visibility = "team"
	}

	autoReview := r.FormValue("auto_review") == "true" || r.FormValue("auto_review") == "1"

	authorID, _ := r.Context().Value(auth.ContextKeyPrincipalID).(uuid.UUID)

	artifact, err := ro.knwStore.Create(r.Context(), knowledge.CreateInput{
		KnowledgeType: knowledgeType,
		OwnerScopeID:  scope.ID,
		AuthorID:      authorID,
		Visibility:    visibility,
		Title:         title,
		Content:       text,
		AutoReview:    autoReview,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, artifact)
}
