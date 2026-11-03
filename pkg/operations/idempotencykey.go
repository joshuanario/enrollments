package operations

import (
	"context"
	"errors"

	"github.com/joshuanario/enrollments/pkg/store"
)

func (o OperationsService) IsIdempotencyKeyUsed(ctx context.Context, eventID string) (bool, error) {
	_, err := o.store.GetEventRow(ctx, eventID)
	return !errors.Is(err, store.ErrNotFound), err
}
