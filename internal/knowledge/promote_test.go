package knowledge

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/simplyblock/postbrain/internal/db"
)

// fakePromoterDB implements promoterDB for unit tests.
type fakePromoterDB struct {
	memory          *db.Memory
	promotionStatus string // tracks the last status set on the memory
	requestCreated  *db.PromotionRequest
}

func (f *fakePromoterDB) getMemory(_ context.Context, _ uuid.UUID) (*db.Memory, error) {
	return f.memory, nil
}

func (f *fakePromoterDB) createPromotionRequest(_ context.Context, req *db.PromotionRequest) (*db.PromotionRequest, error) {
	req.ID = uuid.New()
	f.requestCreated = req
	return req, nil
}

func (f *fakePromoterDB) markMemoryNominated(_ context.Context, _ uuid.UUID) error {
	f.promotionStatus = "nominated"
	if f.memory != nil {
		f.memory.PromotionStatus = "nominated"
	}
	return nil
}

// TestCreateRequest_AlreadyNominated verifies that creating a promotion request
// for an already-nominated memory returns ErrAlreadyPromoted.
func TestCreateRequest_AlreadyNominated(t *testing.T) {
	t.Parallel()
	p := &Promoter{
		dbOps: &fakePromoterDB{
			memory: &db.Memory{
				ID:              uuid.New(),
				PromotionStatus: "nominated",
			},
		},
	}

	_, err := p.CreateRequest(context.Background(), PromoteInput{
		MemoryID:         uuid.New(),
		RequestedBy:      uuid.New(),
		TargetScopeID:    uuid.New(),
		TargetVisibility: "team",
	})
	if !errors.Is(err, ErrAlreadyPromoted) {
		t.Errorf("expected ErrAlreadyPromoted, got %v", err)
	}
}

// TestCreateRequest_AlreadyPromoted verifies that creating a promotion request
// for an already-promoted memory returns ErrAlreadyPromoted.
func TestCreateRequest_AlreadyPromoted(t *testing.T) {
	t.Parallel()
	p := &Promoter{
		dbOps: &fakePromoterDB{
			memory: &db.Memory{
				ID:              uuid.New(),
				PromotionStatus: "promoted",
			},
		},
	}

	_, err := p.CreateRequest(context.Background(), PromoteInput{
		MemoryID:         uuid.New(),
		RequestedBy:      uuid.New(),
		TargetScopeID:    uuid.New(),
		TargetVisibility: "team",
	})
	if !errors.Is(err, ErrAlreadyPromoted) {
		t.Errorf("expected ErrAlreadyPromoted, got %v", err)
	}
}
