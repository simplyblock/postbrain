//go:build integration

package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

	req := httptest.NewRequest(http.MethodPost, "/ui/principals/"+created.ID.String(),
		strings.NewReader("slug=principal-edit-after&display_name=After+Name"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
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
