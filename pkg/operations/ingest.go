package operations

import (
	"context"
	"errors"
	"slices"

	"github.com/joshuanario/enrollments/pkg/store"
	"github.com/joshuanario/enrollments/pkg/web"
	"gorm.io/gorm"
)

// Ingest records each event once and re-evaluates the learner's live,
// incomplete enrollments that include the event's lesson (rules 3, 6, 8).
//
// Each event gets its own result:
//   - invalid events are rejected and not stored, so a corrected redelivery
//     with the same event_id can still be processed (S12);
//   - an event_id already stored is ignored: first write wins, even if the
//     payload differs (S2, S11);
//   - everything else is stored, including activity for lessons that are in
//     none of the learner's paths, which may credit a later enrollment.
//
// The returned error is only for storage failures; events before the failing
// one have already been stored.
func (o OperationsService) Ingest(ctx context.Context, events []web.LessonEvent) ([]web.IngestResultView, error) {
	results := make([]web.IngestResultView, 0, len(events))
	for _, e := range events {
		res := web.IngestResultView{EventID: string(e.EventID)}

		row, reason := validateEvent(e)
		if reason != "" {
			res.Reason = reason
			results = append(results, res)
			continue
		}

		used, err := o.IsIdempotencyKeyUsed(ctx, row.EventID)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return results, err
		}
		if used {
			res.Reason = "duplicate"
			results = append(results, res)
			continue
		}

		if err := o.store.InsertEvent(ctx, &row); err != nil {
			// Another delivery of the same event_id won the race.
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				res.Reason = "duplicate"
				results = append(results, res)
				continue
			}
			return results, err
		}
		if err := o.reevaluateEnrollments(ctx, row.LearnerID, row.LessonKey); err != nil {
			return results, err
		}
		res.Accepted = true
		results = append(results, res)
	}
	return results, nil
}

// validateEvent maps an event to its row, or returns why it is rejected.
func validateEvent(e web.LessonEvent) (store.EventRow, string) {
	switch {
	case e.EventID == "":
		return store.EventRow{}, "missing event_id"
	case e.LearnerID == "":
		return store.EventRow{}, "missing learner_id"
	case e.LessonKey == "":
		return store.EventRow{}, "missing lesson_key"
	case e.Type != "started" && e.Type != "completed":
		return store.EventRow{}, "invalid type"
	}
	occurred, err := fromDateString(e.OccurredAt)
	if err != nil {
		return store.EventRow{}, "invalid occurred_at"
	}
	return store.EventRow{
		EventID:    string(e.EventID),
		LearnerID:  string(e.LearnerID),
		LessonKey:  string(e.LessonKey),
		Type:       e.Type,
		OccurredAt: occurred,
	}, ""
}

// reevaluateEnrollments sets completed_at on each live, incomplete enrollment
// of the learner whose path includes the lesson and is now complete (rule 6).
// Completed enrollments are skipped, so completed_at is never changed.
func (o OperationsService) reevaluateEnrollments(ctx context.Context, learner, lesson string) error {
	enrollments, err := o.store.GetEnrollmentsWithLearnerId(ctx, learner)
	if err != nil {
		return err
	}
	for _, e := range enrollments {
		if e.UnenrolledAt != nil || e.CompletedAt != nil {
			continue
		}
		pathRec, err := o.store.GetPath(ctx, e.PathKey)
		if err != nil {
			return err
		}
		keys := o.pathLessonKeys(pathRec)
		if !slices.Contains(keys, lesson) {
			continue
		}
		if err := o.completeIfDone(ctx, e, keys); err != nil {
			return err
		}
	}
	return nil
}

// completeIfDone sets the enrollment's completed_at if every lesson in keys is
// now complete for it (rule 6). The caller skips enrollments that are already
// complete or unenrolled.
func (o OperationsService) completeIfDone(ctx context.Context, e store.EnrollmentRow, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	events, err := o.store.ListEventsForLearner(ctx, e.LearnerID, keys...)
	if err != nil {
		return err
	}
	_, completedAt := lessonProgress(e, keys, events)
	if completedAt == nil {
		return nil
	}
	return o.store.SetCompletedAt(ctx, e.ID, *completedAt)
}
