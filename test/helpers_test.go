// Package test holds the scenario tests from ASSIGNMENT.md (S1–S14). The
// system under test is operations.OperationsService backed by a real store
// on an in-memory SQLite database; run them with `go test ./test/...`.
package test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/joshuanario/enrollments/pkg/enrollments"
	"github.com/joshuanario/enrollments/pkg/operations"
	"github.com/joshuanario/enrollments/pkg/store"
	"github.com/joshuanario/enrollments/pkg/web"
)

// Timestamps shared by several scenarios.
const (
	aliceFirstEnrolledAt  enrollments.DateString = "2026-03-01T08:00:00Z"
	aliceSecondEnrolledAt enrollments.DateString = "2026-03-12T08:00:00Z"
	assignedAt            enrollments.DateString = "2026-03-01T08:00:00Z"
)

// ---- system under test ----

// newSUT returns a service over a brand-new in-memory SQLite database. Each
// test arranges its own state from scratch, so tests are independent and can
// run in any order.
func newSUT(t *testing.T) (operations.OperationsService, context.Context) {
	t.Helper()
	ctx := context.Background()
	s, err := store.OpenInMemory(ctx)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return operations.NewOperationsService(s), ctx
}

// ---- fixtures ----

type fixtureEvent struct {
	Scenario string `json:"scenario"`
	enrollments.LessonEvent
}

type fixturePath struct {
	Scenario string   `json:"scenario"`
	Key      string   `json:"key"`
	Name     string   `json:"name"`
	Lessons  []string `json:"lessons"`
	Status   string   `json:"status"`
}

func loadFixture(t *testing.T, name string, v any) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("fixtures", name))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatalf("parse fixture %s: %v", name, err)
	}
}

// events returns the fixture events with the given ids, taking each from the
// copy tagged with scenario (event ids repeat across scenarios, e.g. S2, S11).
func events(t *testing.T, scenario string, ids ...string) []enrollments.LessonEvent {
	t.Helper()
	var all []fixtureEvent
	loadFixture(t, "events.json", &all)
	out := make([]enrollments.LessonEvent, 0, len(ids))
	for _, id := range ids {
		found := false
		for _, e := range all {
			if e.Scenario == scenario && string(e.EventID) == id {
				out = append(out, e.LessonEvent)
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("fixture event %s (scenario %s) not found", id, scenario)
		}
	}
	return out
}

// path returns the version of a path fixture tagged with scenario.
func path(t *testing.T, scenario, key string) enrollments.Path {
	t.Helper()
	var all []fixturePath
	loadFixture(t, "paths.json", &all)
	for _, p := range all {
		if p.Scenario != scenario || p.Key != key {
			continue
		}
		out := enrollments.Path{Key: enrollments.PathKey(p.Key), Name: p.Name}
		for _, l := range p.Lessons {
			out.Lessons = append(out.Lessons, enrollments.LessonKey(l))
		}
		switch p.Status {
		case "active":
			out.Status = enrollments.Active
		case "retired":
			out.Status = enrollments.Retired
		}
		return out
	}
	t.Fatalf("fixture path %s (scenario %s) not found", key, scenario)
	return enrollments.Path{}
}

// ---- arrange helpers (fail the test immediately on error) ----

func mustUpsertPaths(t *testing.T, sut operations.OperationsService, ctx context.Context, paths ...enrollments.Path) {
	t.Helper()
	for _, p := range paths {
		if err := sut.UpsertPath(ctx, p); err != nil {
			t.Fatalf("arrange: upsert path %s: %v", p.Key, err)
		}
	}
}

// mustSetupPaths upserts the three "setup" paths: web-basics, secure-go, legacy-path.
func mustSetupPaths(t *testing.T, sut operations.OperationsService, ctx context.Context) {
	t.Helper()
	mustUpsertPaths(t, sut, ctx,
		path(t, "setup", "web-basics"),
		path(t, "setup", "secure-go"),
		path(t, "setup", "legacy-path"))
}

// mustIngest sends each event in its own ingest call, in the order given.
func mustIngest(t *testing.T, sut operations.OperationsService, ctx context.Context, evs ...enrollments.LessonEvent) {
	t.Helper()
	for _, e := range evs {
		if _, err := sut.Ingest(ctx, []enrollments.LessonEvent{e}); err != nil {
			t.Fatalf("arrange: ingest %s: %v", e.EventID, err)
		}
	}
}

func mustEnroll(t *testing.T, sut operations.OperationsService, ctx context.Context, req web.EnrollRequest) web.EnrollmentView {
	t.Helper()
	e, err := sut.Enroll(ctx, req)
	if err != nil {
		t.Fatalf("arrange: enroll %s in %s: %v", req.LearnerID, req.PathKey, err)
	}
	if e.ID == "" {
		t.Fatalf("arrange: enroll %s in %s returned no id", req.LearnerID, req.PathKey)
	}
	return e
}

func mustGet(t *testing.T, sut operations.OperationsService, ctx context.Context, id string) web.EnrollmentView {
	t.Helper()
	e, err := sut.GetEnrollment(ctx, id)
	if err != nil {
		t.Fatalf("get enrollment %s: %v", id, err)
	}
	return e
}

func selfEnroll(learner enrollments.LearnerID, pathKey enrollments.PathKey, at enrollments.DateString) web.EnrollRequest {
	return web.EnrollRequest{LearnerID: learner, PathKey: pathKey, Source: "self", EnrolledAt: at}
}

func assign(learner enrollments.LearnerID, creditDays *int) web.EnrollRequest {
	return web.EnrollRequest{LearnerID: learner, PathKey: "web-basics", Source: "assigned", AssignmentID: "asg-100", CreditDays: creditDays, EnrolledAt: assignedAt}
}

func intPtr(n int) *int { return &n }

// ---- assertion helpers ----

func lesson(t *testing.T, e web.EnrollmentView, key string) web.LessonProgressView {
	t.Helper()
	for _, l := range e.Lessons {
		if l.LessonKey == key {
			return l
		}
	}
	t.Fatalf("enrollment %s has no lesson %s (lessons: %+v)", e.ID, key, e.Lessons)
	return web.LessonProgressView{}
}

func doneCount(e web.EnrollmentView) int {
	n := 0
	for _, l := range e.Lessons {
		if l.CompletedAt != "" {
			n++
		}
	}
	return n
}

func assertProgress(t *testing.T, e web.EnrollmentView, wantDone, wantTotal int) {
	t.Helper()
	if got := doneCount(e); got != wantDone || len(e.Lessons) != wantTotal {
		t.Errorf("progress = %d of %d, want %d of %d (lessons: %+v)", got, len(e.Lessons), wantDone, wantTotal, e.Lessons)
	}
}

func assertCompletedAt(t *testing.T, e web.EnrollmentView, want string) {
	t.Helper()
	if e.CompletedAt != want {
		t.Errorf("enrollment completed_at = %q, want %q", e.CompletedAt, want)
	}
}

func assertLessonComplete(t *testing.T, e web.EnrollmentView, key, wantCompletedAt string, wantCredited bool) {
	t.Helper()
	l := lesson(t, e, key)
	if l.CompletedAt == "" {
		t.Errorf("lesson %s: want complete, got incomplete", key)
		return
	}
	if wantCompletedAt != "" && l.CompletedAt != wantCompletedAt {
		t.Errorf("lesson %s: completed_at = %q, want %q", key, l.CompletedAt, wantCompletedAt)
	}
	if l.Credited != wantCredited {
		t.Errorf("lesson %s: credited = %v, want %v", key, l.Credited, wantCredited)
	}
}

func assertLessonIncomplete(t *testing.T, e web.EnrollmentView, key string) {
	t.Helper()
	if l := lesson(t, e, key); l.CompletedAt != "" {
		t.Errorf("lesson %s: want incomplete, got completed_at %q", key, l.CompletedAt)
	}
}
