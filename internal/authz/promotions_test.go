package authz_test

import (
	"testing"

	"github.com/simplyblock/postbrain/internal/authz"
)

// TestPromotionAccess_DirectPath verifies that holding both memories:write and
// knowledge:write yields PathDirect with the standard review count.
func TestPromotionAccess_DirectPath(t *testing.T) {
	perms, _ := authz.NewPermissionSet([]string{"memories:write", "knowledge:write"})

	kind, reviewRequired, err := authz.PromotionAccess(perms)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if kind != authz.PathDirect {
		t.Errorf("kind = %v, want PathDirect", kind)
	}
	if reviewRequired != authz.StandardReviewRequired {
		t.Errorf("reviewRequired = %d, want StandardReviewRequired (%d)", reviewRequired, authz.StandardReviewRequired)
	}
}

// TestPromotionAccess_DirectPath_WithBroaderPerms verifies that broader permission
// sets (e.g. bare "write") also qualify for the direct path.
func TestPromotionAccess_DirectPath_WithBroaderPerms(t *testing.T) {
	perms, _ := authz.NewPermissionSet([]string{"write"})

	kind, reviewRequired, err := authz.PromotionAccess(perms)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if kind != authz.PathDirect {
		t.Errorf("kind = %v, want PathDirect", kind)
	}
	if reviewRequired != authz.StandardReviewRequired {
		t.Errorf("reviewRequired = %d, want StandardReviewRequired (%d)", reviewRequired, authz.StandardReviewRequired)
	}
}

// TestPromotionAccess_ReviewPath verifies that holding only promotions:write
// (without knowledge:write) yields PathReview with the elevated review count.
func TestPromotionAccess_ReviewPath(t *testing.T) {
	perms, _ := authz.NewPermissionSet([]string{"memories:write", "promotions:write"})

	kind, reviewRequired, err := authz.PromotionAccess(perms)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if kind != authz.PathReview {
		t.Errorf("kind = %v, want PathReview", kind)
	}
	if reviewRequired != authz.ElevatedReviewRequired {
		t.Errorf("reviewRequired = %d, want ElevatedReviewRequired (%d)", reviewRequired, authz.ElevatedReviewRequired)
	}
}

// TestPromotionAccess_ReviewPath_OnlyPromotionsWrite verifies promotions:write alone
// (without memories:write) also qualifies for PathReview.
func TestPromotionAccess_ReviewPath_OnlyPromotionsWrite(t *testing.T) {
	perms, _ := authz.NewPermissionSet([]string{"promotions:write"})

	kind, reviewRequired, err := authz.PromotionAccess(perms)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if kind != authz.PathReview {
		t.Errorf("kind = %v, want PathReview", kind)
	}
	if reviewRequired != authz.ElevatedReviewRequired {
		t.Errorf("reviewRequired = %d, want ElevatedReviewRequired (%d)", reviewRequired, authz.ElevatedReviewRequired)
	}
}

// TestPromotionAccess_Denied verifies that a principal with neither memories:write,
// knowledge:write, nor promotions:write is denied.
func TestPromotionAccess_Denied(t *testing.T) {
	perms, _ := authz.NewPermissionSet([]string{"memories:read", "knowledge:read"})

	_, _, err := authz.PromotionAccess(perms)
	if err == nil {
		t.Error("expected error for principal without promotion permissions, got nil")
	}
}

// TestPromotionAccess_Denied_Empty verifies an empty permission set is denied.
func TestPromotionAccess_Denied_Empty(t *testing.T) {
	_, _, err := authz.PromotionAccess(authz.EmptyPermissionSet())
	if err == nil {
		t.Error("expected error for empty permission set, got nil")
	}
}

// TestReviewCounts_ElevatedExceedsStandard verifies the design invariant that
// ElevatedReviewRequired > StandardReviewRequired.
func TestReviewCounts_ElevatedExceedsStandard(t *testing.T) {
	if authz.ElevatedReviewRequired <= authz.StandardReviewRequired {
		t.Errorf("ElevatedReviewRequired (%d) must be > StandardReviewRequired (%d)",
			authz.ElevatedReviewRequired, authz.StandardReviewRequired)
	}
}

// TestPromotionPathKind_String verifies path kinds have non-empty string values.
func TestPromotionPathKind_String(t *testing.T) {
	if authz.PathDirect == "" {
		t.Error("PathDirect should not be empty string")
	}
	if authz.PathReview == "" {
		t.Error("PathReview should not be empty string")
	}
	if authz.PathDirect == authz.PathReview {
		t.Error("PathDirect and PathReview must be distinct")
	}
}
