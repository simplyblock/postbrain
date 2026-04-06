package authz

import "fmt"

// PromotionPathKind identifies which promotion path applies to a request.
type PromotionPathKind string

const (
	// PathDirect applies when the caller holds both memories:write and knowledge:write.
	// The promotion is created with the standard review threshold.
	PathDirect PromotionPathKind = "direct"

	// PathReview applies when the caller holds promotions:write but not knowledge:write.
	// The promotion is created with an elevated review threshold, requiring additional
	// endorsements before it is merged into knowledge.
	PathReview PromotionPathKind = "review"
)

const (
	// StandardReviewRequired is the review_required value set on a promotion when
	// the caller has direct knowledge:write access.
	StandardReviewRequired = 1

	// ElevatedReviewRequired is the review_required value set on a promotion when
	// the caller only has promotions:write without direct knowledge:write access.
	// More endorsements are required to compensate for the lack of direct authority.
	ElevatedReviewRequired = 3
)

// PromotionAccess determines which promotion path is available to a principal
// and the review_required count that should be set on the resulting promotion_request.
//
// Returns PathDirect + StandardReviewRequired if the caller has both memories:write
// and knowledge:write (or broader equivalents).
//
// Returns PathReview + ElevatedReviewRequired if the caller has promotions:write
// but not knowledge:write.
//
// Returns an error if the caller has neither combination.
func PromotionAccess(callerPermissions PermissionSet) (PromotionPathKind, int, error) {
	hasKnowledgeWrite := callerPermissions.Contains(NewPermission(ResourceKnowledge, OperationWrite))
	hasMemoriesWrite := callerPermissions.Contains(NewPermission(ResourceMemories, OperationWrite))
	hasPromotionsWrite := callerPermissions.Contains(NewPermission(ResourcePromotions, OperationWrite))

	switch {
	case hasMemoriesWrite && hasKnowledgeWrite:
		return PathDirect, StandardReviewRequired, nil
	case hasPromotionsWrite:
		return PathReview, ElevatedReviewRequired, nil
	default:
		return "", 0, fmt.Errorf("authz: principal lacks permission to promote memories: requires memories:write + knowledge:write, or promotions:write")
	}
}
