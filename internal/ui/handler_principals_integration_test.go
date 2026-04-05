//go:build integration

package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/principals"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

func TestHandleUpdatePrincipal_Success(t *testing.T) {
	t.Parallel()
	pool := testhelper.NewTestPool(t)
	h, err := NewHandler(pool, nil)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	ps := principals.NewStore(pool)
	created, err := ps.Create(t.Context(), "user", "principal-edit-before", "Before Name", nil)
	if err != nil {
		t.Fatalf("Create principal: %v", err)
	}
	adminHub, err := ps.Create(t.Context(), "team", "principal-edit-admin-hub-"+uuid.NewString(), "Admin Hub", nil)
	if err != nil {
		t.Fatalf("Create admin hub principal: %v", err)
	}
	actor, err := ps.Create(t.Context(), "user", "principal-edit-actor-"+uuid.NewString(), "Actor", nil)
	if err != nil {
		t.Fatalf("Create actor principal: %v", err)
	}
	ms := principals.NewMembershipStore(pool)
	if err := ms.AddMembership(t.Context(), actor.ID, adminHub.ID, "admin", nil); err != nil {
		t.Fatalf("AddMembership actor admin: %v", err)
	}
	if err := ms.AddMembership(t.Context(), created.ID, adminHub.ID, "member", nil); err != nil {
		t.Fatalf("AddMembership target member: %v", err)
	}

	rawSession, hashSession, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	if _, err := db.CreateToken(t.Context(), pool, actor.ID, hashSession, "ui-principal-edit-session", nil, nil, nil); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/ui/principals/"+created.ID.String(),
		strings.NewReader("slug=principal-edit-after&display_name=After+Name"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: cookieName, Value: rawSession})
	w := httptest.NewRecorder()

	h.handleUpdatePrincipal(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusSeeOther)
	}
	if got := w.Header().Get("Location"); got != "/ui/principals" {
		t.Fatalf("Location = %q, want %q", got, "/ui/principals")
	}

	updated, err := ps.GetByID(t.Context(), created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if updated == nil {
		t.Fatal("expected updated principal, got nil")
	}
	if updated.Slug != "principal-edit-after" {
		t.Fatalf("slug = %q, want %q", updated.Slug, "principal-edit-after")
	}
	if updated.DisplayName != "After Name" {
		t.Fatalf("display_name = %q, want %q", updated.DisplayName, "After Name")
	}
}
