package store

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrNotFound = errors.New("not found")

func notFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}

// ---- paths ----

// PathRecord is a path row together with its lesson rows. The store returns
// lessons in insertion (id) order; putting them in path order is up to the
// caller's LessonSorting.
type PathRecord struct {
	Path    PathRow
	Lessons []LessonRow
}

// UpsertPath creates or updates a path and the given lesson rows in place.
// A lesson with ID 0 is inserted and its new ID written back into p.Lessons;
// any other lesson row is updated. Nothing is deleted: lesson rows left out of
// p.Lessons stay as they are.
func (s *Store) UpsertPath(ctx context.Context, p PathRecord) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&p.Path).Error; err != nil {
			return err
		}
		for i := range p.Lessons {
			if err := tx.Omit(clause.Associations).Save(&p.Lessons[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) GetPath(ctx context.Context, key string) (PathRecord, error) {
	var rec PathRecord
	if err := s.db.WithContext(ctx).First(&rec.Path, "key = ?", key).Error; err != nil {
		return PathRecord{}, notFound(err)
	}
	if err := s.db.WithContext(ctx).Where("path_key = ?", key).Order("id").Find(&rec.Lessons).Error; err != nil {
		return PathRecord{}, err
	}
	return rec, nil
}

func (s *Store) ListPaths(ctx context.Context) ([]PathRecord, error) {
	var rows []PathRow
	if err := s.db.WithContext(ctx).Order("key").Find(&rows).Error; err != nil {
		return nil, err
	}
	var lessons []LessonRow
	if err := s.db.WithContext(ctx).Order("path_key, id").Find(&lessons).Error; err != nil {
		return nil, err
	}
	byPath := make(map[string][]LessonRow, len(rows))
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
	err := s.db.WithContext(ctx).Model(&LessonRow{}).
		Distinct("path_key").Where("lesson_key = ?", lesson).
		Order("path_key").Pluck("path_key", &keys).Error
	return keys, err
}

// GetLessonRows returns the path's lesson rows whose lesson_key is one of
// keys, in id order. Keys with no rows are simply absent from the result.
func (s *Store) GetLessonRows(ctx context.Context, pathKey string, keys []string) ([]LessonRow, error) {
	if len(keys) == 0 {
		return []LessonRow{}, nil
	}
	var rows []LessonRow
	err := s.db.WithContext(ctx).Where("path_key = ? AND lesson_key IN ?", pathKey, keys).Order("id").Find(&rows).Error
	return rows, err
}

// CreateLessonRows inserts the rows in one statement and writes their new IDs
// back into rows. Only the rows' own columns are written, not Path or
// NextLesson. An empty slice is a no-op.
func (s *Store) CreateLessonRows(ctx context.Context, rows []LessonRow) error {
	if len(rows) == 0 {
		return nil
	}
	return s.db.WithContext(ctx).Omit(clause.Associations).Create(&rows).Error
}

// UpsertLessonRows makes sure the path has its own lesson row (no learner) for
// each key, creating the missing ones, and returns the rows in the order of
// lessonKeys. Existing rows are returned as stored; nothing is updated or
// deleted, and ordering columns (Index, Order, NextId) are left to the caller.
func (s *Store) UpsertLessonRows(ctx context.Context, pathKey string, lessonKeys []string) ([]LessonRow, error) {
	if len(lessonKeys) == 0 {
		return []LessonRow{}, nil
	}
	var out []LessonRow
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing []LessonRow
		if err := tx.Where("path_key = ? AND learner_id = ? AND lesson_key IN ?", pathKey, "", lessonKeys).
			Find(&existing).Error; err != nil {
			return err
		}
		byKey := make(map[string]LessonRow, len(existing))
		for _, r := range existing {
			byKey[r.LessonKey] = r
		}
		var missing []LessonRow
		for _, k := range lessonKeys {
			if _, ok := byKey[k]; !ok {
				missing = append(missing, LessonRow{PathKey: pathKey, LessonKey: k})
				byKey[k] = LessonRow{} // a repeated key is created once
			}
		}
		if len(missing) > 0 {
			if err := tx.Omit(clause.Associations).Create(&missing).Error; err != nil {
				return err
			}
			for _, r := range missing {
				byKey[r.LessonKey] = r
			}
		}
		out = make([]LessonRow, 0, len(lessonKeys))
		seen := make(map[string]bool, len(lessonKeys))
		for _, k := range lessonKeys {
			if !seen[k] {
				seen[k] = true
				out = append(out, byKey[k])
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// UpdateLessonRows writes each row's Status and positioning fields (NextId,
// Order, Index) by ID, in one transaction. nil and empty values are written
// too, so they clear the column. Other columns are left as stored. Returns
// ErrNotFound, and updates nothing, if any row's ID does not exist.
func (s *Store) UpdateLessonRows(ctx context.Context, rows []LessonRow) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, r := range rows {
			res := tx.Model(&LessonRow{}).Where("id = ?", r.ID).Updates(map[string]any{
				"status":  r.Status,
				"next_id": r.NextId,
				"order":   r.Order,
				"index":   r.Index,
			})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return ErrNotFound
			}
		}
		return nil
	})
}

// ---- enrollments ----

// CreateEnrollment inserts the row and fills in its ID.
func (s *Store) CreateEnrollment(ctx context.Context, e *EnrollmentRow) error {
	tx := s.db.WithContext(ctx).Create(e)
	return tx.Error
}

func (s *Store) GetEnrollment(ctx context.Context, id int64) (EnrollmentRow, error) {
	var row EnrollmentRow
	if err := s.db.WithContext(ctx).First(&row, id).Error; err != nil {
		return EnrollmentRow{}, notFound(err)
	}
	return row, nil
}

// GetEnrollmentWithLearnerAndPath returns the learner's most recent enrollment
// in the path (latest enrolled_at, then highest id), live or not. Check
// UnenrolledAt to tell which. Returns ErrNotFound if there is none.
func (s *Store) GetEnrollmentWithLearnerAndPath(ctx context.Context, learner, path string) (EnrollmentRow, error) {
	var row EnrollmentRow
	err := s.db.WithContext(ctx).Where("learner_id = ? AND path_key = ?", learner, path).
		Order("enrolled_at DESC, id DESC").First(&row).Error
	if err != nil {
		return EnrollmentRow{}, notFound(err)
	}
	return row, nil
}

// GetEnrollmentsWithLearnerId returns the learner's live and historical
// enrollments across all paths, oldest first.
func (s *Store) GetEnrollmentsWithLearnerId(ctx context.Context, learner string) ([]EnrollmentRow, error) {
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

// UpdateEnrollment overwrites every column of the row with e.ID, including
// nil ones (so a nil CompletedAt clears it). Returns ErrNotFound if no row
// has that ID.
func (s *Store) UpdateEnrollment(ctx context.Context, e *EnrollmentRow) error {
	res := s.db.WithContext(ctx).Model(&EnrollmentRow{}).Where("id = ?", e.ID).
		Select("*").Omit("id").Updates(e)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
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

// GetEventRow returns the stored row with that event_id, or ErrNotFound.
func (s *Store) GetEventRow(ctx context.Context, eventID string) (EventRow, error) {
	var row EventRow
	if err := s.db.WithContext(ctx).First(&row, "event_id = ?", eventID).Error; err != nil {
		return EventRow{}, notFound(err)
	}
	return row, nil
}

// ListEventsForLearner returns the learner's events ordered by occurred_at,
// optionally limited to the given lessons.
func (s *Store) ListEventsForLearner(ctx context.Context, learner string, lessons ...string) ([]EventRow, error) {
	q := s.db.WithContext(ctx).Where("learner_id = ?", learner)
	if len(lessons) > 0 {
		q = q.Where("lesson_key IN ?", lessons)
	}
	var rows []EventRow
	err := q.Order("occurred_at, id").Find(&rows).Error
	return rows, err
}
