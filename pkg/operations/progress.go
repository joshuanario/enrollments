package operations

import (
	"context"
	"strconv"
	"time"

	"github.com/joshuanario/enrollments/pkg/store"
	"github.com/joshuanario/enrollments/pkg/web"
)

// enrollmentView builds the enrollment's view with lesson progress computed
// from the learner's events (rules 4 and 5) over the path's current lessons.
func (o OperationsService) enrollmentView(ctx context.Context, e store.EnrollmentRow, pathRec store.PathRecord) (web.EnrollmentView, error) {
	keys := o.pathLessonKeys(pathRec)
	var events []store.EventRow
	if len(keys) > 0 {
		var err error
		events, err = o.store.ListEventsForLearner(ctx, e.LearnerID, keys...)
		if err != nil {
			return web.EnrollmentView{}, err
		}
	}
	lessons, _ := lessonProgress(e, keys, events)
	view := web.EnrollmentView{
		ID:           strconv.FormatInt(e.ID, 10),
		LearnerID:    e.LearnerID,
		PathKey:      e.PathKey,
		Source:       e.Source,
		CreditDays:   e.CreditDays,
		EnrolledAt:   toDateString(e.EnrolledAt),
		CompletedAt:  toOptionalDateString(e.CompletedAt),
		UnenrolledAt: toOptionalDateString(e.UnenrolledAt),
		Lessons:      lessons,
	}
	if e.AssignmentID != nil {
		view.AssignmentID = *e.AssignmentID
	}
	return view, nil
}

// pathLessonKeys returns the path's lesson keys in path order. Only the path's
// own lesson rows (no learner) define the list.
func (o OperationsService) pathLessonKeys(rec store.PathRecord) []string {
	rows := make([]store.LessonRow, 0, len(rec.Lessons))
	for _, l := range rec.Lessons {
		if l.LearnerID == "" {
			rows = append(rows, l)
		}
	}
	if len(rows) == 0 {
		return nil
	}
	switch *o.sortingStrategy {
	case FractionalIndexing:
		rows, _ = SortLessonsAsFractionalIndexing(rows)
	case IntegerPositionColumn:
		rows, _ = SortLessonsAsIntegerPositionColumn(rows)
	case AdjacencyList:
		rows, _ = SortLessonsAsAdjacencyList(rows)
	}
	keys := make([]string, 0)
	for _, l := range rows {
		if l.Status == "removed" {
			continue
		}
		keys = append(keys, l.LessonKey)
	}
	return keys
}

// creditCutoff is the earliest occurred_at that counts for the enrollment
// (rule 4), or nil for no cutoff.
func creditCutoff(e store.EnrollmentRow) *time.Time {
	if e.Source != "assigned" || e.CreditDays == nil {
		return nil
	}
	t := e.EnrolledAt.AddDate(0, 0, -*e.CreditDays)
	return &t
}

// lessonProgress computes per-lesson progress for the enrollment from the
// learner's events (rules 4 and 5). It depends only on the set of events, not
// their arrival order. completedAt is non-nil when every lesson is complete:
// the latest lesson completion, or enrolled_at if that is later (i.e. every
// lesson was credited) (rule 6).
func lessonProgress(e store.EnrollmentRow, keys []string, events []store.EventRow) (lessons []web.LessonProgressView, completedAt *time.Time) {
	cutoff := creditCutoff(e)
	var latest time.Time
	complete := len(keys) > 0
	lessons = make([]web.LessonProgressView, 0, len(keys))
	for _, key := range keys {
		var started, completed *time.Time
		for _, ev := range events {
			if ev.LessonKey != key || (cutoff != nil && ev.OccurredAt.Before(*cutoff)) {
				continue
			}
			at := ev.OccurredAt
			if started == nil || at.Before(*started) {
				started = &at
			}
			if ev.Type == "completed" && (completed == nil || at.Before(*completed)) {
				completed = &at
			}
		}
		v := web.LessonProgressView{LessonKey: key, CompletedAt: toOptionalDateString(completed)}
		if started != nil {
			v.StartedAt = toDateString(*started)
		}
		if completed != nil {
			v.Credited = completed.Before(e.EnrolledAt)
			if completed.After(latest) {
				latest = *completed
			}
		} else {
			complete = false
		}
		lessons = append(lessons, v)
	}
	if !complete {
		return lessons, nil
	}
	if latest.Before(e.EnrolledAt) {
		latest = e.EnrolledAt
	}
	return lessons, &latest
}
