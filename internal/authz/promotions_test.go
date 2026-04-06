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

// TestPromotionAccess_DirectPath_WhenBothPathsAvailable verifies that PathDirect
// takes precedence over PathReview when the caller holds both knowledge:write and
// promotions:write.
func TestPromotionAccess_DirectPath_WhenBothPathsAvailable(t *testing.T) {
	perms, _ := authz.NewPermissionSet([]string{"memories:write", "knowledge:write", "promotions:write"})

	kind, _, err := authz.PromotionAccess(perms)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if kind != authz.PathDirect {
		t.Errorf("kind = %v, want PathDirect (direct takes precedence over review)", kind)
	}
}

// TestPromotionAccess_MemberRole verifies that a member role holder (who has
// memories:write and knowledge:write) is granted PathDirect.
func TestPromotionAccess_MemberRole(t *testing.T) {
	perms := authz.RolePermissions(authz.RoleMember)

	kind, reviewRequired, err := authz.PromotionAccess(perms)
	if err != nil {
		t.Fatalf("RoleMember: unexpected error: %v", err)
	}
	if kind != authz.PathDirect {
		t.Errorf("RoleMember: kind = %v, want PathDirect", kind)
	}
	if reviewRequired != authz.StandardReviewRequired {
		t.Errorf("RoleMember: reviewRequired = %d, want %d", reviewRequired, authz.StandardReviewRequired)
	}
}

// TestPromotionAccess_AdminRole verifies that admin role permissions qualify for PathDirect.
func TestPromotionAccess_AdminRole(t *testing.T) {
	perms := authz.RolePermissions(authz.RoleAdmin)

	kind, _, err := authz.PromotionAccess(perms)
	if err != nil {
		t.Fatalf("RoleAdmin: unexpected error: %v", err)
	}
	if kind != authz.PathDirect {
		t.Errorf("RoleAdmin: kind = %v, want PathDirect", kind)
	}
}

// TestPromotionAccess_OwnerRole verifies that owner role permissions qualify for PathDirect.
func TestPromotionAccess_OwnerRole(t *testing.T) {
	perms := authz.RolePermissions(authz.RoleOwner)

	kind, _, err := authz.PromotionAccess(perms)
	if err != nil {
		t.Fatalf("RoleOwner: unexpected error: %v", err)
	}
	if kind != authz.PathDirect {
		t.Errorf("RoleOwner: kind = %v, want PathDirect", kind)
	}
}

// TestStandardReviewRequired_AtLeastOne verifies the standard review count is >= 1.
func TestStandardReviewRequired_AtLeastOne(t *testing.T) {
	if authz.StandardReviewRequired < 1 {
		t.Errorf("StandardReviewRequired = %d, must be >= 1", authz.StandardReviewRequired)
	}
}

// TestElevatedReviewRequired_AtLeastTwo verifies the elevated review count is >= 2,
// ensuring it is meaningfully higher than standard.
func TestElevatedReviewRequired_AtLeastTwo(t *testing.T) {
	if authz.ElevatedReviewRequired < 2 {
		t.Errorf("ElevatedReviewRequired = %d, must be >= 2 to be meaningfully elevated", authz.ElevatedReviewRequired)
	}
}

// TestPromotionAccess_Denied_OnlyPromotionsRead verifies that promotions:read alone
// is not sufficient to promote (write is required).
func TestPromotionAccess_Denied_OnlyPromotionsRead(t *testing.T) {
	perms, _ := authz.NewPermissionSet([]string{"promotions:read"})

	_, _, err := authz.PromotionAccess(perms)
	if err == nil {
		t.Error("expected error for principal with only promotions:read, got nil")
	}
}

// TestPromotionAccess_Denied_KnowledgeWriteAlone verifies that knowledge:write
// alone (without memories:write) is not sufficient for PathDirect.
func TestPromotionAccess_Denied_KnowledgeWriteAlone(t *testing.T) {
	perms, _ := authz.NewPermissionSet([]string{"knowledge:write"})

	_, _, err := authz.PromotionAccess(perms)
	if err == nil {
		t.Error("expected error for principal with only knowledge:write, got nil")
	}
}

// TestPromotionAccess_Denied_OnlyMemoriesWrite verifies that memories:write alone
// (without knowledge:write or promotions:write) is not sufficient to promote.
func TestPromotionAccess_Denied_OnlyMemoriesWrite(t *testing.T) {
	perms, _ := authz.NewPermissionSet([]string{"memories:write"})

	_, _, err := authz.PromotionAccess(perms)
	if err == nil {
		t.Error("expected error for principal with only memories:write (no knowledge:write), got nil")
	}
}

// TestPromotionAccess_ReviewPath_ReturnedCount verifies that PathReview always
// returns exactly ElevatedReviewRequired.
func TestPromotionAccess_ReviewPath_ReturnedCount(t *testing.T) {
	cases := [][]string{
		{"promotions:write"},
		{"memories:write", "promotions:write"},
		{"memories:read", "promotions:write"},
	}
	for _, raw := range cases {
		perms, _ := authz.NewPermissionSet(raw)
		kind, count, err := authz.PromotionAccess(perms)
		if err != nil {
			t.Errorf("PromotionAccess(%v): unexpected error: %v", raw, err)
			continue
		}
		if kind != authz.PathReview {
			continue // only check count for PathReview cases
		}
		if count != authz.ElevatedReviewRequired {
			t.Errorf("PromotionAccess(%v): reviewRequired = %d, want ElevatedReviewRequired (%d)",
				raw, count, authz.ElevatedReviewRequired)
		}
	}
}
