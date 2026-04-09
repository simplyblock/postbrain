package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/simplyblock/postbrain/internal/authz"
)

func TestHandleUpdatePrincipal_InvalidID_RendersError(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/ui/principals/not-a-uuid",
		strings.NewReader("slug=alice&display_name=Alice"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	h.handleUpdatePrincipal(w, req)

	if w.Code >= http.StatusInternalServerError {
		t.Fatalf("expected non-5xx status, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "invalid principal id") {
		t.Fatalf("expected invalid principal id error in response body")
	}
}

func TestHandleUpdatePrincipal_MissingFields_RendersError(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	id := uuid.New()
	req := httptest.NewRequest(http.MethodPost, "/ui/principals/"+id.String(),
		strings.NewReader("slug=alice"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	h.handleUpdatePrincipal(w, req)

	if w.Code >= http.StatusInternalServerError {
		t.Fatalf("expected non-5xx status, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "slug and display_name are required") {
		t.Fatalf("expected missing fields error in response body")
	}
}

func TestHandleUpdatePrincipal_NilPool_RendersError(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	id := uuid.New()
	req := httptest.NewRequest(http.MethodPost, "/ui/principals/"+id.String(),
		strings.NewReader("slug=alice&display_name=Alice"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	h.handleUpdatePrincipal(w, req)

	if w.Code >= http.StatusInternalServerError {
		t.Fatalf("expected non-5xx status, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "service unavailable") {
		t.Fatalf("expected service unavailable in response body")
	}
}

func TestHandlePrincipals_RendersEditDialog(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/ui/principals", nil)
	w := httptest.NewRecorder()

	h.handlePrincipals(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "id=\"dlg-principal-edit\"") {
		t.Fatalf("expected edit dialog in principals page")
	}
}

func TestParseScopeGrantPermissionsInput_ResourceAndAdvanced(t *testing.T) {
	t.Parallel()

	perms, err := parseScopeGrantPermissionsInput([]string{
		"collections",
		"skills:read",
		"skills:write",
	})
	if err != nil {
		t.Fatalf("parseScopeGrantPermissionsInput: %v", err)
	}

	collectionsOps := authz.ValidOperations(authz.ResourceCollections)
	for _, op := range collectionsOps {
		p := authz.NewPermission(authz.ResourceCollections, op)
		if !perms.Contains(p) {
			t.Fatalf("expected expanded collections permission %q", p)
		}
	}
	if !perms.Contains(authz.NewPermission(authz.ResourceSkills, authz.OperationRead)) {
		t.Fatal("expected skills:read")
	}
	if !perms.Contains(authz.NewPermission(authz.ResourceSkills, authz.OperationWrite)) {
		t.Fatal("expected skills:write")
	}
	for _, p := range perms.Permissions() {
		if p == authz.NewPermission(authz.ResourceSkills, authz.OperationEdit) ||
			p == authz.NewPermission(authz.ResourceSkills, authz.OperationDelete) {
			t.Fatalf("unexpected advanced expansion for skills: %q", p)
		}
	}
}

func TestParseScopeGrantPermissionsInput_RejectsUnknownResource(t *testing.T) {
	t.Parallel()

	_, err := parseScopeGrantPermissionsInput([]string{"not_a_resource"})
	if err == nil {
		t.Fatal("expected error for unknown resource")
	}
	if !strings.Contains(err.Error(), "unknown resource") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHandlePrincipals_RendersScopeGrantGroupedPermissions(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/ui/principals", nil)
	w := httptest.NewRecorder()

	h.handlePrincipals(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "id=\"dlg-scope-grant\"") {
		t.Fatal("expected scope grant dialog")
	}
	if !strings.Contains(body, "data-scope-grant-resource=\"collections\"") {
		t.Fatal("expected resource-level scope grant selector")
	}
	if !strings.Contains(body, "name=\"permissions_adv\"") {
		t.Fatal("expected advanced permission checkboxes")
	}
	if !strings.Contains(body, "name=\"permissions_basic\"") {
		t.Fatal("expected basic resource permission checkboxes")
	}
}

func TestHandlePrincipals_ScopeGrantPicker_HidesAdvancedLabel(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/ui/principals", nil)
	w := httptest.NewRecorder()

	h.handlePrincipals(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}
	body := w.Body.String()
	if strings.Contains(body, ">Advanced<") {
		t.Fatal("did not expect explicit Advanced label in scope grant picker")
	}
}

func TestHandlePrincipals_ScopeGrantPicker_UsesToggleAndCenteredPopoverHooks(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/ui/principals", nil)
	w := httptest.NewRecorder()

	h.handlePrincipals(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "class=\"scope-grant-toggle\"") {
		t.Fatal("expected scope-grant toggle class for open/close indicator")
	}
	if !strings.Contains(body, "class=\"scope-grant-ops-popover\"") {
		t.Fatal("expected scope-grant centered popover class")
	}
	if !strings.Contains(body, "class=\"scope-grant-permissions\"") {
		t.Fatal("expected scope-grant permissions container class")
	}
}
