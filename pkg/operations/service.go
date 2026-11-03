package operations

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/joshuanario/enrollments/pkg/store"
	"github.com/joshuanario/enrollments/pkg/web"
)

var ErrOperationRejected = errors.New("rejected")

type SortingStrategy int

const (
	FractionalIndexing SortingStrategy = iota
	IntegerPositionColumn
	AdjacencyList
)

type OperationsService struct {
	store           *store.Store
	sortingStrategy *SortingStrategy
}

func NewOperationsService(s *store.Store, ss *SortingStrategy) OperationsService {
	return OperationsService{store: s, sortingStrategy: ss}
}

func (o OperationsService) UpsertPath(ctx context.Context, path web.Path) error {
	lessonKeys := make([]string, len(path.Lessons))
	for i, l := range path.Lessons {
		lessonKeys[i] = string(l)
	}
	lessons, err := o.store.GetLessonRows(ctx, string(path.Key), lessonKeys)
	if err != nil {
		return err
	}
	rec := store.PathRecord{
		Path: store.PathRow{
			Key:    string(path.Key),
			Name:   path.Name,
			Status: string(path.Status),
		},
		Lessons: lessons,
	}
	pathRecord, err := o.store.GetPath(ctx, string(path.Key))
	if errors.Is(err, store.ErrNotFound) || err == nil {
		err = o.store.UpsertPath(ctx, rec)
		if err != nil {
			return err
		}
		historicalLessons := pathRecord.Lessons
		upsertedLessons, err := o.store.UpsertLessonRows(ctx, string(path.Key), lessonKeys)
		if err != nil {
			return err
		}
		var restored []store.LessonRow
		for i := range upsertedLessons {
			if upsertedLessons[i].Status == "removed" {
				upsertedLessons[i].Status = ""
				restored = append(restored, upsertedLessons[i])
			}
		}
		updatedLessons := make([]store.LessonRow, 0)
		switch *o.sortingStrategy {
		case FractionalIndexing:
			rec.Lessons, updatedLessons = SortLessonsAsFractionalIndexing(upsertedLessons)
		case IntegerPositionColumn:
			rec.Lessons, updatedLessons = SortLessonsAsIntegerPositionColumn(upsertedLessons)
		case AdjacencyList:
			rec.Lessons, updatedLessons = SortLessonsAsAdjacencyList(upsertedLessons)
		default:
			rec.Lessons = upsertedLessons
		}
		for _, r := range restored {
			if !slices.ContainsFunc(updatedLessons, func(u store.LessonRow) bool { return u.ID == r.ID }) {
				updatedLessons = append(updatedLessons, r)
			}
		}
		for _, l := range historicalLessons {
			historicalLessonKey := l.LessonKey
			if !slices.ContainsFunc(rec.Lessons, func(liveLesson store.LessonRow) bool {
				return liveLesson.LessonKey == historicalLessonKey
			}) {
				l.Status = "removed"
				updatedLessons = append(updatedLessons, l)
			}
		}
		err = o.store.UpdateLessonRows(ctx, updatedLessons)
		if err != nil {
			return err
		}
		err = o.reevaluatePathEnrollments(ctx, string(path.Key))
	}
	return err
}

// reevaluatePathEnrollments completes each live, incomplete enrollment in the
// path that the path's current lessons now make complete (rule 7). Completed
// enrollments are skipped, so completed_at is never changed (rule 6).
func (o OperationsService) reevaluatePathEnrollments(ctx context.Context, pathKey string) error {
	pathRec, err := o.store.GetPath(ctx, pathKey)
	if err != nil {
		return err
	}
	keys := o.pathLessonKeys(pathRec)
	enrollments, err := o.store.ListLiveEnrollmentsByPath(ctx, pathKey)
	if err != nil {
		return err
	}
	for _, e := range enrollments {
		if e.CompletedAt != nil {
			continue
		}
		if err := o.completeIfDone(ctx, e, keys); err != nil {
			return err
		}
	}
	return nil
}
// ListPaths returns every path, ordered by key, with its current lessons in
// path order (removed lessons left out).
func (o OperationsService) ListPaths(ctx context.Context) ([]web.Path, error) {
	recs, err := o.store.ListPaths(ctx)
	if err != nil {
		return nil, err
	}
	paths := make([]web.Path, 0, len(recs))
	for _, rec := range recs {
		p := web.Path{
			Key:     web.PathKey(rec.Path.Key),
			Name:    rec.Path.Name,
			Status:  web.PathStatus(rec.Path.Status),
			Lessons: []web.LessonKey{},
		}
		for _, k := range o.pathLessonKeys(rec) {
			p.Lessons = append(p.Lessons, web.LessonKey(k))
		}
		paths = append(paths, p)
	}
	return paths, nil
}
func (o OperationsService) Enroll(ctx context.Context, request web.EnrollRequest) (web.EnrollmentView, error) {
	pathRec, err := o.store.GetPath(ctx, string(request.PathKey))
	if err != nil {
		return web.EnrollmentView{}, err
	}
	if pathRec.Path.Status == string(web.PathRetired) {
		return web.EnrollmentView{}, ErrOperationRejected // rule 2
	}
	historical, err := o.store.GetEnrollmentWithLearnerAndPath(ctx, string(request.LearnerID), string(request.PathKey))
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return web.EnrollmentView{}, err
	}
	if err == nil && historical.UnenrolledAt == nil {
		return web.EnrollmentView{}, ErrOperationRejected // rule 1: one live enrollment
	}

	enrolledAt := time.Now().UTC()
	if request.EnrolledAt != "" {
		enrolledAt, err = fromDateString(request.EnrolledAt)
		if err != nil {
			return web.EnrollmentView{}, err
		}
	}
	// Re-enrolling after an unenroll creates a new row; history is kept (rule 1).
	row := &store.EnrollmentRow{
		LearnerID:  string(request.LearnerID),
		PathKey:    string(request.PathKey),
		Source:     request.Source,
		CreditDays: request.CreditDays,
		EnrolledAt: enrolledAt,
	}
	if request.AssignmentID != "" {
		row.AssignmentID = &request.AssignmentID
	}
	if err := o.store.CreateEnrollment(ctx, row); err != nil {
		return web.EnrollmentView{}, err
	}

	// Prior activity can credit or even complete the enrollment right away
	// (rules 4, 6, 8).
	keys := o.pathLessonKeys(pathRec)
	if len(keys) > 0 {
		events, err := o.store.ListEventsForLearner(ctx, row.LearnerID, keys...)
		if err != nil {
			return web.EnrollmentView{}, err
		}
		if _, completedAt := lessonProgress(*row, keys, events); completedAt != nil {
			if err := o.store.SetCompletedAt(ctx, row.ID, *completedAt); err != nil {
				return web.EnrollmentView{}, err
			}
			row.CompletedAt = completedAt
		}
	}
	return o.enrollmentView(ctx, *row, pathRec)
}
func (o OperationsService) Unenroll(ctx context.Context, request web.UnenrollRequest) error {
	historical, err := o.store.GetEnrollmentWithLearnerAndPath(ctx, string(request.LearnerID), string(request.PathKey))
	if err != nil && errors.Is(err, store.ErrNotFound) {
		return err
	}
	if historical.UnenrolledAt == nil {
		t, err := fromDateString(request.UnenrolledAt)
		if err != nil {
			return err
		}
		historical.UnenrolledAt = &t
		err = o.store.UpdateEnrollment(ctx, &historical)
		if err != nil {
			return err
		}
	}
	return nil
}
func (o OperationsService) GetEnrollment(ctx context.Context, idString string) (web.EnrollmentView, error) {
	id, err := strconv.ParseInt(idString, 10, 64)
	if err != nil {
		return web.EnrollmentView{}, err
	}
	historical, err := o.store.GetEnrollment(ctx, id)
	if err != nil {
		return web.EnrollmentView{}, err
	}
	pathRec, err := o.store.GetPath(ctx, historical.PathKey)
	if err != nil {
		return web.EnrollmentView{}, err
	}
	return o.enrollmentView(ctx, historical, pathRec)
}
func (o OperationsService) ListLearnerEnrollments(ctx context.Context, learnerID web.LearnerID) ([]web.EnrollmentView, error) {
	historicals, err := o.store.GetEnrollmentsWithLearnerId(ctx, string(learnerID))
	if err != nil {
		return nil, err
	}
	paths := make(map[string]store.PathRecord) // a learner's enrollments often share a path
	views := make([]web.EnrollmentView, 0, len(historicals))
	for _, historical := range historicals {
		pathRec, ok := paths[historical.PathKey]
		if !ok {
			pathRec, err = o.store.GetPath(ctx, historical.PathKey)
			if err != nil {
				return nil, err
			}
			paths[historical.PathKey] = pathRec
		}
		view, err := o.enrollmentView(ctx, historical, pathRec)
		if err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, nil
}

// toDateString formats t as an RFC 3339 UTC DateString, e.g. EnrolledAt.
func toDateString(t time.Time) web.DateString {
	return web.DateString(t.UTC().Format(time.RFC3339))
}

// fromDateString parses an RFC 3339 DateString, e.g. UnenrolledAt, into a UTC
// time. An empty or malformed value is an error.
func fromDateString(d web.DateString) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, string(d))
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid timestamp %q: %w", d, err)
	}
	return t.UTC(), nil
}

// toOptionalDateString is toDateString for nullable columns such as
// CompletedAt and UnenrolledAt; nil becomes "".
func toOptionalDateString(t *time.Time) web.DateString {
	if t == nil {
		return ""
	}
	return toDateString(*t)
}
