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
	if err := s.DB().Exec("TRUNCATE paths, path_lessons, lessons, enrollments, activity_events RESTART IDENTITY").Error; err != nil {
		t.Fatal(err)
	}
	return s
}

func TestPathRoundTripAndReplace(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	rec := PathRecord{
		Path: PathRow{Key: "web-basics", Name: "Web Basics", Status: "active"},
		Lessons: []PathLessonRow{
			{PathKey: "web-basics", Position: 0, LessonKey: "L1"},
			{PathKey: "web-basics", Position: 1, LessonKey: "L2"},
			{PathKey: "web-basics", Position: 2, LessonKey: "L3"},
		},
	}
	if err := s.UpsertPath(ctx, rec); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetPath(ctx, "web-basics")
	if err != nil {
		t.Fatal(err)
	}
	if got.Path.Name != "Web Basics" || got.Path.Status != "active" || !reflect.DeepEqual(got.Lessons, rec.Lessons) {
		t.Fatalf("got %+v, want %+v", got, rec)
	}

	rec.Path.Status = "retired"
	rec.Lessons = []PathLessonRow{
		{PathKey: "web-basics", Position: 0, LessonKey: "L3"},
		{PathKey: "web-basics", Position: 1, LessonKey: "L1"},
	}
	if err := s.UpsertPath(ctx, rec); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetPath(ctx, "web-basics")
	if got.Path.Status != "retired" || !reflect.DeepEqual(got.Lessons, rec.Lessons) {
		t.Fatalf("after replace got %+v, want %+v", got, rec)
	}

	all, err := s.ListPaths(ctx)
	if err != nil || len(all) != 1 || len(all[0].Lessons) != 2 {
		t.Fatalf("ListPaths got %+v %v", all, err)
	}

	keys, err := s.PathsContainingLesson(ctx, "L2")
	if err != nil || len(keys) != 0 {
		t.Fatalf("L2 should be in no path, got %v %v", keys, err)
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
	e := EnrollmentRow{LearnerID: "carol", PathKey: "web-basics", Source: "assigned", AssignmentID: &aid, CreditDays: &days, EnrolledAt: enrolled}
	if err := s.CreateEnrollment(ctx, &e); err != nil {
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
	all, _ := s.ListEnrollmentsByLearner(ctx, "carol")
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
	e := EventRow{EventID: "e1", LessonKey: "L2", Type: "completed", OccurredAt: occurred}
	if err := s.InsertEvent(ctx, &e); err != nil {
		t.Fatal(err)
	}
	if e.ID == 0 {
		t.Fatal("expected ID to be assigned")
	}

	got, err := s.FindEventByEventID(ctx, "e1")
	if err != nil {
		t.Fatal(err)
	}
	if got.LessonKey != "L2" || got.Type != "completed" || !got.OccurredAt.Equal(occurred) {
		t.Fatalf("got %+v", got)
	}

	dup := EventRow{EventID: "e1", LessonKey: "L2", Type: "completed", OccurredAt: occurred.Add(48 * time.Hour)}
	if err := s.InsertEvent(ctx, &dup); err == nil {
		t.Fatal("expected unique violation on repeated event_id")
	}

	if _, err := s.FindEventByEventID(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}
