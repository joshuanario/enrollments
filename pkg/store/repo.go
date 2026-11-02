package store

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

var ErrNotFound = errors.New("not found")

func notFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}

// ---- paths ----

// PathRecord is a path row together with its lessons, ordered by position.
type PathRecord struct {
	Path    PathRow
	Lessons []PathLessonRow
}

// UpsertPath creates or replaces a path and its full lesson list.
func (s *Store) UpsertPath(ctx context.Context, p PathRecord) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&p.Path).Error; err != nil {
			return err
		}
		if err := tx.Where("path_key = ?", p.Path.Key).Delete(&PathLessonRow{}).Error; err != nil {
			return err
		}
		if len(p.Lessons) == 0 {
			return nil
		}
		return tx.Create(&p.Lessons).Error
	})
}

func (s *Store) GetPath(ctx context.Context, key string) (PathRecord, error) {
	var rec PathRecord
	if err := s.db.WithContext(ctx).First(&rec.Path, "key = ?", key).Error; err != nil {
		return PathRecord{}, notFound(err)
	}
	if err := s.db.WithContext(ctx).Where("path_key = ?", key).Order("position").Find(&rec.Lessons).Error; err != nil {
		return PathRecord{}, err
	}
	return rec, nil
}

func (s *Store) ListPaths(ctx context.Context) ([]PathRecord, error) {
	var rows []PathRow
	if err := s.db.WithContext(ctx).Order("key").Find(&rows).Error; err != nil {
		return nil, err
	}
	var lessons []PathLessonRow
	if err := s.db.WithContext(ctx).Order("path_key, position").Find(&lessons).Error; err != nil {
		return nil, err
	}
	byPath := make(map[string][]PathLessonRow, len(rows))
	for _, l := range lessons {
		byPath[l.PathKey] = append(byPath[l.PathKey], l)
	}
	out := make([]PathRecord, len(rows))
	for i, r := range rows {
		out[i] = PathRecord{Path: r, Lessons: byPath[r.Key]}
	}
	return out, nil
}

// PathsContainingLesson returns the keys of paths whose current lesson list
// includes the lesson.
func (s *Store) PathsContainingLesson(ctx context.Context, lesson string) ([]string, error) {
	var keys []string
	err := s.db.WithContext(ctx).Model(&PathLessonRow{}).
		Distinct("path_key").Where("lesson_key = ?", lesson).
		Order("path_key").Pluck("path_key", &keys).Error
	return keys, err
}

// ---- enrollments ----

// CreateEnrollment inserts the row and fills in its ID.
func (s *Store) CreateEnrollment(ctx context.Context, e *EnrollmentRow) error {
	return s.db.WithContext(ctx).Create(e).Error
}

func (s *Store) GetEnrollment(ctx context.Context, id int64) (EnrollmentRow, error) {
	var row EnrollmentRow
	if err := s.db.WithContext(ctx).First(&row, id).Error; err != nil {
		return EnrollmentRow{}, notFound(err)
	}
	return row, nil
}

// ListEnrollmentsByLearner returns live and historical enrollments, oldest first.
func (s *Store) ListEnrollmentsByLearner(ctx context.Context, learner string) ([]EnrollmentRow, error) {
	var rows []EnrollmentRow
	err := s.db.WithContext(ctx).Where("learner_id = ?", learner).
		Order("enrolled_at, id").Find(&rows).Error
	return rows, err
}

func (s *Store) ListLiveEnrollmentsByPath(ctx context.Context, path string) ([]EnrollmentRow, error) {
	var rows []EnrollmentRow
	err := s.db.WithContext(ctx).Where("path_key = ? AND unenrolled_at IS NULL", path).
		Order("id").Find(&rows).Error
	return rows, err
}

func (s *Store) SetCompletedAt(ctx context.Context, id int64, at time.Time) error {
	return s.setColumn(ctx, id, "completed_at", at.UTC())
}

func (s *Store) SetUnenrolledAt(ctx context.Context, id int64, at time.Time) error {
	return s.setColumn(ctx, id, "unenrolled_at", at.UTC())
}

func (s *Store) setColumn(ctx context.Context, id int64, col string, v any) error {
	res := s.db.WithContext(ctx).Model(&EnrollmentRow{}).Where("id = ?", id).Update(col, v)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- activity events ----

// InsertEvent stores the row and fills in its ID. A repeated event_id fails
// on the unique index; what that means is up to the caller.
func (s *Store) InsertEvent(ctx context.Context, e *EventRow) error {
	return s.db.WithContext(ctx).Create(e).Error
}

// FindEventByEventID returns the stored row with that event_id.
func (s *Store) FindEventByEventID(ctx context.Context, id string) (EventRow, error) {
	var row EventRow
	if err := s.db.WithContext(ctx).First(&row, "event_id = ?", id).Error; err != nil {
		return EventRow{}, notFound(err)
	}
	return row, nil
}

// ListEventsForLearner returns the learner's events ordered by occurred_at,
// optionally limited to the given lessons.
//
// NOTE: activity_events currently has no learner_id column (see EventRow),
// so this query fails until one is added back.
func (s *Store) ListEventsForLearner(ctx context.Context, learner string, lessons ...string) ([]EventRow, error) {
	q := s.db.WithContext(ctx).Where("learner_id = ?", learner)
	if len(lessons) > 0 {
		q = q.Where("lesson_key IN ?", lessons)
	}
	var rows []EventRow
	err := q.Order("occurred_at, id").Find(&rows).Error
	return rows, err
}
