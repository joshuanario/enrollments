package store

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"
)

// openTestStore uses a fresh in-memory SQLite database, or, when DATABASE_URL
// is set, connects to it and truncates all tables.
func openTestStore(t *testing.T) *Store {
	t.Helper()
	ctx := context.Background()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		s, err := OpenInMemory(ctx)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { s.Close() })
		return s
	}
	s, err := Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.DB().Exec("TRUNCATE paths, lessons, enrollments, activity_events RESTART IDENTITY").Error; err != nil {
		t.Fatal(err)
	}
	return s
}

func lessonKeys(rows []LessonRow) []string {
	keys := make([]string, len(rows))
	for i, r := range rows {
		keys[i] = r.LessonKey
	}
	return keys
}

func TestPathRoundTripAndReplace(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	rec := PathRecord{
		Path: PathRow{Key: "web-basics", Name: "Web Basics", Status: "active"},
		Lessons: []LessonRow{
			{PathKey: "web-basics", LessonKey: "L1"},
			{PathKey: "web-basics", LessonKey: "L2"},
			{PathKey: "web-basics", LessonKey: "L3"},
		},
	}
	if err := s.UpsertPath(ctx, rec); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetPath(ctx, "web-basics")
	if err != nil {
		t.Fatal(err)
	}
	if got.Path.Name != "Web Basics" || got.Path.Status != "active" ||
		!reflect.DeepEqual(lessonKeys(got.Lessons), []string{"L1", "L2", "L3"}) {
		t.Fatalf("got %+v", got)
	}

	// Second upsert: update L1 in place, add L4, leave L2 and L3 out.
	l1 := got.Lessons[0]
	l1.Status = "completed"
	rec.Path.Status = "retired"
	rec.Lessons = []LessonRow{l1, {PathKey: "web-basics", LessonKey: "L4"}}
	if err := s.UpsertPath(ctx, rec); err != nil {
		t.Fatal(err)
	}
	if rec.Lessons[1].ID == 0 {
		t.Fatal("expected new lesson ID to be written back")
	}
	got, _ = s.GetPath(ctx, "web-basics")
	if got.Path.Status != "retired" || !reflect.DeepEqual(lessonKeys(got.Lessons), []string{"L1", "L2", "L3", "L4"}) {
		t.Fatalf("after update got %+v", got)
	}
	if got.Lessons[0].ID != l1.ID || got.Lessons[0].Status != "completed" {
		t.Fatalf("L1 not updated in place: %+v", got.Lessons[0])
	}

	all, err := s.ListPaths(ctx)
	if err != nil || len(all) != 1 || len(all[0].Lessons) != 4 {
		t.Fatalf("ListPaths got %+v %v", all, err)
	}

	keys, err := s.PathsContainingLesson(ctx, "L2")
	if err != nil || !reflect.DeepEqual(keys, []string{"web-basics"}) {
		t.Fatalf("L2 should still be in web-basics, got %v %v", keys, err)
	}

	if _, err := s.GetPath(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestEnrollmentRoundTrip(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	aid, days := "asg-1", 30
	enrolled := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	var e EnrollmentRow
	e.LearnerID = "carol"
	e.PathKey = "web-basics"
	e.Source = "assigned"
	e.AssignmentID = &aid
	e.CreditDays = &days
	e.EnrolledAt = enrolled
	err := s.CreateEnrollment(ctx, &e)
	if err != nil {
		t.Fatal(err)
	}
	if e.ID == 0 {
		t.Fatal("expected ID to be assigned")
	}

	done := time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC)
	if err := s.SetCompletedAt(ctx, e.ID, done); err != nil {
		t.Fatal(err)
	}
	if err := s.SetUnenrolledAt(ctx, e.ID, done.Add(24*time.Hour)); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetEnrollment(ctx, e.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.EnrolledAt.Equal(enrolled) || got.CompletedAt == nil || !got.CompletedAt.Equal(done) ||
		got.UnenrolledAt == nil || *got.CreditDays != 30 || *got.AssignmentID != "asg-1" {
		t.Fatalf("unexpected row %+v", got)
	}

	live, _ := s.ListLiveEnrollmentsByPath(ctx, "web-basics")
	if len(live) != 0 {
		t.Fatalf("unenrolled row should not be live, got %d", len(live))
	}
	all, _ := s.GetEnrollmentsWithLearnerId(ctx, "carol")
	if len(all) != 1 {
		t.Fatalf("want 1 enrollment, got %d", len(all))
	}

	if err := s.SetCompletedAt(ctx, 999, done); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestEventRoundTrip(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	occurred := time.Date(2026, 3, 5, 14, 0, 0, 0, time.UTC)
	e := EventRow{EventID: "e1", LearnerID: "alice", LessonKey: "L2", Type: "completed", OccurredAt: occurred}
	if err := s.InsertEvent(ctx, &e); err != nil {
		t.Fatal(err)
	}
	if e.ID == 0 {
		t.Fatal("expected ID to be assigned")
	}

	got, err := s.GetEventRow(ctx, "e1")
	if err != nil {
		t.Fatal(err)
	}
	if got.LessonKey != "L2" || got.Type != "completed" || !got.OccurredAt.Equal(occurred) {
		t.Fatalf("got %+v", got)
	}

	dup := EventRow{EventID: "e1", LearnerID: "alice", LessonKey: "L2", Type: "completed", OccurredAt: occurred.Add(48 * time.Hour)}
	if err := s.InsertEvent(ctx, &dup); err == nil {
		t.Fatal("expected unique violation on repeated event_id")
	}

	if _, err := s.GetEventRow(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestGetEnrollmentWithLearnerAndPath(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	first := EnrollmentRow{LearnerID: "alice", PathKey: "web-basics", Source: "self", EnrolledAt: time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC)}
	if err := s.CreateEnrollment(ctx, &first); err != nil {
		t.Fatal(err)
	}
	if err := s.SetUnenrolledAt(ctx, first.ID, time.Date(2026, 3, 11, 8, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	second := EnrollmentRow{LearnerID: "alice", PathKey: "web-basics", Source: "self", EnrolledAt: time.Date(2026, 3, 12, 8, 0, 0, 0, time.UTC)}
	if err := s.CreateEnrollment(ctx, &second); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetEnrollmentWithLearnerAndPath(ctx, "alice", "web-basics")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != second.ID || got.UnenrolledAt != nil {
		t.Fatalf("want latest enrollment %d, got %+v", second.ID, got)
	}

	if _, err := s.GetEnrollmentWithLearnerAndPath(ctx, "alice", "secure-go"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestUpdateEnrollment(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	done := time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC)
	e := EnrollmentRow{LearnerID: "bob", PathKey: "web-basics", Source: "self", EnrolledAt: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), CompletedAt: &done}
	if err := s.CreateEnrollment(ctx, &e); err != nil {
		t.Fatal(err)
	}

	aid, days := "asg-9", 14
	e.Source, e.AssignmentID, e.CreditDays = "assigned", &aid, &days
	e.CompletedAt = nil
	if err := s.UpdateEnrollment(ctx, &e); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetEnrollment(ctx, e.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != "assigned" || got.AssignmentID == nil || *got.AssignmentID != "asg-9" ||
		got.CreditDays == nil || *got.CreditDays != 14 || got.CompletedAt != nil || got.LearnerID != "bob" {
		t.Fatalf("unexpected row after update %+v", got)
	}

	missing := EnrollmentRow{ID: 999, LearnerID: "x", PathKey: "y", Source: "self", EnrolledAt: done}
	if err := s.UpdateEnrollment(ctx, &missing); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestGetLessonRows(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	rec := PathRecord{
		Path: PathRow{Key: "web-basics", Name: "Web Basics", Status: "active"},
		Lessons: []LessonRow{
			{PathKey: "web-basics", LessonKey: "L1"},
			{PathKey: "web-basics", LessonKey: "L2"},
			{PathKey: "web-basics", LessonKey: "L3"},
		},
	}
	if err := s.UpsertPath(ctx, rec); err != nil {
		t.Fatal(err)
	}

	other := PathRecord{
		Path:    PathRow{Key: "secure-go", Name: "Secure Go", Status: "active"},
		Lessons: []LessonRow{{PathKey: "secure-go", LessonKey: "L1", LearnerID: "other"}},
	}
	if err := s.UpsertPath(ctx, other); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetLessonRows(ctx, "web-basics", []string{"L3", "L1", "nope"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(lessonKeys(got), []string{"L1", "L3"}) || got[0].PathKey != "web-basics" {
		t.Fatalf("got %+v, want web-basics L1 and L3 only", got)
	}

	none, err := s.GetLessonRows(ctx, "web-basics", nil)
	if err != nil || len(none) != 0 {
		t.Fatalf("empty keys: got %v %v", none, err)
	}
}

func TestCreateLessonRows(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	if err := s.UpsertPath(ctx, PathRecord{Path: PathRow{Key: "web-basics", Name: "Web Basics", Status: "active"}}); err != nil {
		t.Fatal(err)
	}
	i1, i2 := 1.0, 2.0
	rows := []LessonRow{
		{PathKey: "web-basics", LessonKey: "L1", Index: &i1},
		{PathKey: "web-basics", LessonKey: "L2", Index: &i2},
	}
	if err := s.CreateLessonRows(ctx, rows); err != nil {
		t.Fatal(err)
	}
	if rows[0].ID == 0 || rows[1].ID == 0 {
		t.Fatalf("expected IDs written back, got %+v", rows)
	}
	got, err := s.GetPath(ctx, "web-basics")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(lessonKeys(got.Lessons), []string{"L1", "L2"}) {
		t.Fatalf("got %v", lessonKeys(got.Lessons))
	}

	if err := s.CreateLessonRows(ctx, nil); err != nil {
		t.Fatalf("empty insert: %v", err)
	}
	if err := s.CreateLessonRows(ctx, []LessonRow{{PathKey: "nope", LessonKey: "X"}}); err == nil {
		t.Fatal("expected foreign key error for unknown path")
	}
}

func TestUpsertLessonRows(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	if err := s.UpsertPath(ctx, PathRecord{Path: PathRow{Key: "web-basics", Name: "Web Basics", Status: "active"}}); err != nil {
		t.Fatal(err)
	}
	first, err := s.UpsertLessonRows(ctx, "web-basics", []string{"L1", "L2"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(lessonKeys(first), []string{"L1", "L2"}) || first[0].ID == 0 || first[1].ID == 0 {
		t.Fatalf("first upsert got %+v", first)
	}

	// L2 already exists and keeps its ID; L3 is new; order follows the input.
	second, err := s.UpsertLessonRows(ctx, "web-basics", []string{"L3", "L2", "L3"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(lessonKeys(second), []string{"L3", "L2"}) || second[1].ID != first[1].ID || second[0].ID == 0 {
		t.Fatalf("second upsert got %+v", second)
	}

	got, _ := s.GetPath(ctx, "web-basics")
	if len(got.Lessons) != 3 {
		t.Fatalf("want 3 rows (nothing deleted), got %v", lessonKeys(got.Lessons))
	}
}

func TestUpdateLessonRows(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	if err := s.UpsertPath(ctx, PathRecord{Path: PathRow{Key: "web-basics", Name: "Web Basics", Status: "active"}}); err != nil {
		t.Fatal(err)
	}
	rows, err := s.UpsertLessonRows(ctx, "web-basics", []string{"L1", "L2"})
	if err != nil {
		t.Fatal(err)
	}

	// Set every positioning field, then clear L2's Index again.
	o1, o2 := uint(0), uint(1)
	i1, i2 := 1.0, 2.5
	rows[0].Order, rows[0].Index, rows[0].NextId = &o1, &i1, &rows[1].ID
	rows[1].Order, rows[1].Index = &o2, &i2
	rows[1].LessonKey = "ignored" // not a positioning field
	if err := s.UpdateLessonRows(ctx, rows); err != nil {
		t.Fatal(err)
	}
	rows[1].Index = nil
	rows[1].Status = "removed"
	if err := s.UpdateLessonRows(ctx, rows[1:]); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetLessonRows(ctx, "web-basics", []string{"L1", "L2"})
	if err != nil {
		t.Fatal(err)
	}
	l1, l2 := got[0], got[1]
	if l1.Order == nil || *l1.Order != 0 || l1.Index == nil || *l1.Index != 1.0 || l1.NextId == nil || *l1.NextId != l2.ID {
		t.Fatalf("L1 positioning not stored: %+v", l1)
	}
	if l1.Status != "" {
		t.Fatalf("L1 status = %q, want empty", l1.Status)
	}
	if l2.Order == nil || *l2.Order != 1 || l2.Index != nil || l2.NextId != nil || l2.LessonKey != "L2" || l2.Status != "removed" {
		t.Fatalf("L2 unexpected: %+v", l2)
	}

	if err := s.UpdateLessonRows(ctx, []LessonRow{{ID: 999}}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}
