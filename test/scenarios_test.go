package test

import (
	"context"
	"reflect"
	"testing"

	"github.com/joshuanario/enrollments/pkg/operations"
	"github.com/joshuanario/enrollments/pkg/web"
)

// ---- shared arrangements: each replays the earlier scenarios a test builds on ----

// arrangeAliceS1: Alice completed L1 on 2025-11-15, then self-enrolled in
// web-basics on 2026-03-01. Returns her enrollment.
func arrangeAliceS1(t *testing.T, sut operations.OperationsService, ctx context.Context) web.EnrollmentView {
	t.Helper()
	mustSetupPaths(t, sut, ctx)
	mustIngest(t, sut, ctx, events(t, "S1", "evt-s1-01")...)
	return mustEnroll(t, sut, ctx, selfEnroll("alice", "web-basics", aliceFirstEnrolledAt))
}

// arrangeAliceS3: S1 plus Alice's L2 events (delivered out of order).
func arrangeAliceS3(t *testing.T, sut operations.OperationsService, ctx context.Context) web.EnrollmentView {
	t.Helper()
	e := arrangeAliceS1(t, sut, ctx)
	mustIngest(t, sut, ctx, events(t, "S3", "evt-s3-01", "evt-s3-02")...)
	return e
}

// arrangeAliceS5: S3 plus Alice's L3 events; her first enrollment is complete.
func arrangeAliceS5(t *testing.T, sut operations.OperationsService, ctx context.Context) web.EnrollmentView {
	t.Helper()
	e := arrangeAliceS3(t, sut, ctx)
	mustIngest(t, sut, ctx, events(t, "S5", "evt-s5-01", "evt-s5-02")...)
	return e
}

// arrangeS7 brings secure-go to (G1, G2) as after S6, enrolls Gina (January)
// and Hana (2026-03-03), and ingests Gina's completions so she is complete
// under (G1, G2). Returns Gina's and Hana's enrollments.
func arrangeS7(t *testing.T, sut operations.OperationsService, ctx context.Context) (gina, hana web.EnrollmentView) {
	t.Helper()
	mustSetupPaths(t, sut, ctx)
	mustUpsertPaths(t, sut, ctx, path(t, "S6", "secure-go"))
	gina = mustEnroll(t, sut, ctx, selfEnroll("gina", "secure-go", "2026-01-10T08:00:00Z"))
	hana = mustEnroll(t, sut, ctx, selfEnroll("hana", "secure-go", "2026-03-03T08:00:00Z"))
	mustIngest(t, sut, ctx, events(t, "S7", "evt-s7-01", "evt-s7-02", "evt-s7-03")...)
	return gina, hana
}

// arrangeHanaS7: S7 through its end. secure-go is (G1..G4) and Hana is 3 of 4.
func arrangeHanaS7(t *testing.T, sut operations.OperationsService, ctx context.Context) (gina, hana web.EnrollmentView) {
	t.Helper()
	gina, hana = arrangeS7(t, sut, ctx)
	mustUpsertPaths(t, sut, ctx, path(t, "S7", "secure-go"))
	mustIngest(t, sut, ctx, events(t, "S7", "evt-s7-04", "evt-s7-05", "evt-s7-06")...)
	return gina, hana
}

// arrangeIvanS10: Ivan's orphan Q7 completion, then quickstart (Q7, Q8) is
// created and Ivan self-enrolls.
func arrangeIvanS10(t *testing.T, sut operations.OperationsService, ctx context.Context) web.EnrollmentView {
	t.Helper()
	mustSetupPaths(t, sut, ctx)
	mustIngest(t, sut, ctx, events(t, "S10", "evt-s10-01")...)
	mustUpsertPaths(t, sut, ctx, path(t, "S10", "quickstart"))
	return mustEnroll(t, sut, ctx, selfEnroll("ivan", "quickstart", "2026-03-15T08:00:00Z"))
}

// ---- scenarios ----

// S1, credit on self enrollment.
func TestS1_CreditOnSelfEnrollment(t *testing.T) {
	// Arrange
	sut, ctx := newSUT(t)
	mustSetupPaths(t, sut, ctx)
	mustIngest(t, sut, ctx, events(t, "S1", "evt-s1-01")...)

	// Act
	got, err := sut.Enroll(ctx, selfEnroll("alice", "web-basics", aliceFirstEnrolledAt))

	// Assert
	if err != nil {
		t.Fatalf("enroll: %v", err)
	}
	assertLessonComplete(t, got, "L1", "2025-11-15T09:00:00Z", true)
	assertLessonIncomplete(t, got, "L2")
	assertLessonIncomplete(t, got, "L3")
	assertProgress(t, got, 1, 3)
	assertCompletedAt(t, got, "")
}

// S2, duplicate delivery: the same event_id again changes nothing.
func TestS2_DuplicateDelivery(t *testing.T) {
	// Arrange
	sut, ctx := newSUT(t)
	alice := arrangeAliceS1(t, sut, ctx)
	before := mustGet(t, sut, ctx, alice.ID)

	// Act
	_, err := sut.Ingest(ctx, events(t, "S2", "evt-s1-01"))

	// Assert
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	after := mustGet(t, sut, ctx, alice.ID)
	if !reflect.DeepEqual(after, before) {
		t.Errorf("enrollment changed after duplicate delivery:\n before %+v\n after  %+v", before, after)
	}
	assertLessonComplete(t, after, "L1", "2025-11-15T09:00:00Z", true)
	assertCompletedAt(t, after, "")
}

// S3, out-of-order delivery: L2 completed arrives before L2 started.
func TestS3_OutOfOrderDelivery(t *testing.T) {
	// Arrange
	sut, ctx := newSUT(t)
	alice := arrangeAliceS1(t, sut, ctx)
	completed := events(t, "S3", "evt-s3-01") // occurred 2026-03-05
	started := events(t, "S3", "evt-s3-02")   // occurred 2026-03-04

	// Act
	_, errCompleted := sut.Ingest(ctx, completed)
	_, errStarted := sut.Ingest(ctx, started)

	// Assert
	if errCompleted != nil || errStarted != nil {
		t.Fatalf("ingest: completed=%v started=%v", errCompleted, errStarted)
	}
	got := mustGet(t, sut, ctx, alice.ID)
	l2 := lesson(t, got, "L2")
	if l2.StartedAt != "2026-03-04T09:00:00Z" {
		t.Errorf("L2 started_at = %q, want 2026-03-04T09:00:00Z", l2.StartedAt)
	}
	assertLessonComplete(t, got, "L2", "2026-03-05T14:00:00Z", false)
	assertCompletedAt(t, got, "")
}

// S4, credit window on assigned enrollments.
func TestS4_CreditWindowOnAssignedEnrollments(t *testing.T) {
	cases := []struct {
		name       string
		learner    web.LearnerID
		creditDays *int
		wantL1     bool
	}{
		{"bob credit_days 0", "bob", intPtr(0), false},
		{"carol credit_days 30", "carol", intPtr(30), true},
		{"dave credit_days null", "dave", nil, true},
		{"erin credit_days 30, completed 2024", "erin", intPtr(30), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			sut, ctx := newSUT(t)
			mustSetupPaths(t, sut, ctx)
			mustIngest(t, sut, ctx, events(t, "S4", "evt-s4-01", "evt-s4-02", "evt-s4-03", "evt-s4-04")...)

			// Act
			got, err := sut.Enroll(ctx, assign(tc.learner, tc.creditDays))

			// Assert
			if err != nil {
				t.Fatalf("enroll: %v", err)
			}
			if tc.wantL1 {
				assertLessonComplete(t, got, "L1", "", true)
			} else {
				assertLessonIncomplete(t, got, "L1")
			}
		})
	}
}

// S5, enrollment completion: L1 credited, L2 done, L3 completes the path.
func TestS5_EnrollmentCompletion(t *testing.T) {
	// Arrange
	sut, ctx := newSUT(t)
	alice := arrangeAliceS3(t, sut, ctx)

	// Act
	mustIngest(t, sut, ctx, events(t, "S5", "evt-s5-01", "evt-s5-02")...)

	// Assert
	got := mustGet(t, sut, ctx, alice.ID)
	l3 := lesson(t, got, "L3")
	if l3.StartedAt != "2026-03-09T11:00:00Z" {
		t.Errorf("L3 started_at = %q, want 2026-03-09T11:00:00Z", l3.StartedAt)
	}
	assertLessonComplete(t, got, "L3", "2026-03-10T16:00:00Z", false)
	assertProgress(t, got, 3, 3)
	assertCompletedAt(t, got, "2026-03-10T16:00:00Z")
}

// S6, lesson removed from a path completes Frank's enrollment.
func TestS6_LessonRemovedFromPath(t *testing.T) {
	// Arrange
	sut, ctx := newSUT(t)
	mustSetupPaths(t, sut, ctx)
	frank := mustEnroll(t, sut, ctx, selfEnroll("frank", "secure-go", "2026-03-01T08:00:00Z"))
	mustIngest(t, sut, ctx, events(t, "S6", "evt-s6-01", "evt-s6-02")...)
	if before := mustGet(t, sut, ctx, frank.ID); doneCount(before) != 2 || before.CompletedAt != "" {
		t.Fatalf("arrange: want frank 2 of 3 and incomplete, got %+v", before)
	}

	// Act
	err := sut.UpsertPath(ctx, path(t, "S6", "secure-go")) // (G1, G2)

	// Assert
	if err != nil {
		t.Fatalf("upsert path: %v", err)
	}
	got := mustGet(t, sut, ctx, frank.ID)
	assertProgress(t, got, 2, 2)
	assertCompletedAt(t, got, "2026-03-03T10:00:00Z")
}

// S7, lesson added to a path (plus the variant with an old G4 completion).
func TestS7_LessonAddedToPath(t *testing.T) {
	t.Run("gina stays complete, hana is 3 of 4", func(t *testing.T) {
		// Arrange
		sut, ctx := newSUT(t)
		gina, hana := arrangeS7(t, sut, ctx)
		if g := mustGet(t, sut, ctx, gina.ID); g.CompletedAt != "2026-01-22T10:00:00Z" {
			t.Fatalf("arrange: want gina complete at 2026-01-22T10:00:00Z, got %q", g.CompletedAt)
		}

		// Act
		err := sut.UpsertPath(ctx, path(t, "S7", "secure-go")) // (G1, G2, G3, G4)
		mustIngest(t, sut, ctx, events(t, "S7", "evt-s7-04", "evt-s7-05", "evt-s7-06")...)

		// Assert
		if err != nil {
			t.Fatalf("upsert path: %v", err)
		}
		assertCompletedAt(t, mustGet(t, sut, ctx, gina.ID), "2026-01-22T10:00:00Z")
		h := mustGet(t, sut, ctx, hana.ID)
		assertProgress(t, h, 3, 4)
		assertCompletedAt(t, h, "")
	})

	t.Run("variant: old G4 completion credits hana", func(t *testing.T) {
		// Arrange
		sut, ctx := newSUT(t)
		_, hana := arrangeHanaS7(t, sut, ctx)

		// Act
		_, err := sut.Ingest(ctx, events(t, "S7-variant", "evt-s7-07")) // G4 on 2025-05-05

		// Assert
		if err != nil {
			t.Fatalf("ingest: %v", err)
		}
		h := mustGet(t, sut, ctx, hana.ID)
		assertLessonComplete(t, h, "G4", "2025-05-05T10:00:00Z", true)
		assertProgress(t, h, 4, 4)
		assertCompletedAt(t, h, "2026-03-06T10:00:00Z")
	})
}

// S8, re-enroll after unenroll.
func TestS8_ReEnrollAfterUnenroll(t *testing.T) {
	// Arrange
	sut, ctx := newSUT(t)
	first := arrangeAliceS5(t, sut, ctx)

	// Act
	errUnenroll := sut.Unenroll(ctx, web.UnenrollRequest{LearnerID: web.LearnerID(first.LearnerID), PathKey: web.PathKey(first.PathKey), UnenrolledAt: "2026-03-11T08:00:00Z"})
	second, errEnroll := sut.Enroll(ctx, selfEnroll("alice", "web-basics", aliceSecondEnrolledAt))

	// Assert
	if errUnenroll != nil || errEnroll != nil {
		t.Fatalf("unenroll=%v enroll=%v", errUnenroll, errEnroll)
	}
	list, err := sut.ListLearnerEnrollments(ctx, "alice")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("want 2 enrollments, got %d: %+v", len(list), list)
	}
	byID := map[string]web.EnrollmentView{list[0].ID: list[0], list[1].ID: list[1]}

	old, ok := byID[first.ID]
	if !ok {
		t.Fatalf("first enrollment %s missing from list", first.ID)
	}
	assertCompletedAt(t, old, "2026-03-10T16:00:00Z")
	if old.UnenrolledAt != "2026-03-11T08:00:00Z" {
		t.Errorf("first unenrolled_at = %q, want 2026-03-11T08:00:00Z", old.UnenrolledAt)
	}

	cur, ok := byID[second.ID]
	if !ok {
		t.Fatalf("second enrollment %s missing from list", second.ID)
	}
	for _, key := range []string{"L1", "L2", "L3"} {
		assertLessonComplete(t, cur, key, "", true)
	}
	assertCompletedAt(t, cur, string(aliceSecondEnrolledAt))
	if cur.UnenrolledAt != "" {
		t.Errorf("second enrollment should be live, unenrolled_at = %q", cur.UnenrolledAt)
	}
}

// S9, rejections: a second live enrollment, and a retired path.
func TestS9_Rejections(t *testing.T) {
	// Arrange
	sut, ctx := newSUT(t)
	mustSetupPaths(t, sut, ctx)
	mustEnroll(t, sut, ctx, selfEnroll("alice", "web-basics", aliceSecondEnrolledAt))

	// Act
	_, errLive := sut.Enroll(ctx, selfEnroll("alice", "web-basics", "2026-03-13T08:00:00Z"))
	_, errRetired := sut.Enroll(ctx, selfEnroll("bob", "legacy-path", "2026-03-13T08:00:00Z"))

	// Assert
	if errLive == nil {
		t.Error("enrolling alice while live in web-basics: want rejection, got nil")
	}
	if errRetired == nil {
		t.Error("enrolling bob in retired legacy-path: want rejection, got nil")
	}
	if list, _ := sut.ListLearnerEnrollments(ctx, "alice"); len(list) != 1 {
		t.Errorf("alice: want 1 enrollment after rejection, got %d", len(list))
	}
	if list, _ := sut.ListLearnerEnrollments(ctx, "bob"); len(list) != 0 {
		t.Errorf("bob: want no enrollments after rejection, got %d", len(list))
	}
}

// S10, orphan activity credits a path created later.
func TestS10_OrphanActivity(t *testing.T) {
	// Arrange
	sut, ctx := newSUT(t)
	mustSetupPaths(t, sut, ctx)
	mustIngest(t, sut, ctx, events(t, "S10", "evt-s10-01")...) // Q7 is in no path yet
	mustUpsertPaths(t, sut, ctx, path(t, "S10", "quickstart"))

	// Act
	got, err := sut.Enroll(ctx, selfEnroll("ivan", "quickstart", "2026-03-15T08:00:00Z"))

	// Assert
	if err != nil {
		t.Fatalf("enroll: %v", err)
	}
	assertLessonComplete(t, got, "Q7", "2026-02-15T10:00:00Z", true)
	assertLessonIncomplete(t, got, "Q8")
	assertCompletedAt(t, got, "")
}

// S11, repeated id with a different payload (evt-s3-01 now occurred 2026-03-07).
func TestS11_RepeatedIDDifferentPayload(t *testing.T) {
	// Arrange
	sut, ctx := newSUT(t)
	alice := arrangeAliceS3(t, sut, ctx)
	before := mustGet(t, sut, ctx, alice.ID)

	// Act
	results, err := sut.Ingest(ctx, events(t, "S11", "evt-s3-01"))

	// Assert
	after := mustGet(t, sut, ctx, alice.ID)
	t.Logf("ingest err=%v results=%+v; L2 before=%+v after=%+v", err, results, lesson(t, before, "L2"), lesson(t, after, "L2"))
	// TODO(you): the spec doesn't say what happens. Options:
	//   - first write wins: err == nil, result rejected (e.g. "duplicate"),
	//     L2 completed_at stays 2026-03-05T14:00:00Z (rule 3: no additional effect)
	//   - last write wins: L2 completed_at becomes 2026-03-07T14:00:00Z
	//   - conflict: result rejected with a distinct reason, payload kept for audit
	t.Skip("TODO(you): assert the S11 outcome you choose")
}

// S12, mixed batch: one call with a valid event, an unknown type and a bad timestamp.
func TestS12_MixedBatch(t *testing.T) {
	// Arrange
	sut, ctx := newSUT(t)
	ivan := arrangeIvanS10(t, sut, ctx)
	batch := events(t, "S12", "evt-s12-01", "evt-s12-02", "evt-s12-03")

	// Act
	results, err := sut.Ingest(ctx, batch) // a single call

	// Assert
	got := mustGet(t, sut, ctx, ivan.ID)
	t.Logf("ingest err=%v results=%+v; Q8=%+v", err, results, lesson(t, got, "Q8"))
	// TODO(you): the spec doesn't say what happens. Options:
	//   - per-event: err == nil, 3 results; evt-s12-01 accepted (Q8 started_at
	//     2026-03-16T10:00:00Z, still incomplete), evt-s12-02 and evt-s12-03
	//     rejected with reasons (invalid type, invalid timestamp)
	//   - all-or-nothing: err != nil (or all rejected), Q8 untouched
	t.Skip("TODO(you): assert the S12 outcome you choose")
}

// S13, retire secure-go while Hana is live and incomplete.
func TestS13_RetireWithLearnersInFlight(t *testing.T) {
	// Arrange
	sut, ctx := newSUT(t)
	_, hana := arrangeHanaS7(t, sut, ctx)

	// Act
	err := sut.UpsertPath(ctx, path(t, "S13", "secure-go")) // status retired

	// Assert
	got := mustGet(t, sut, ctx, hana.ID)
	t.Logf("upsert err=%v; hana=%+v", err, got)
	// TODO(you): the spec doesn't say what happens. Options:
	//   - retire allowed, Hana stays live and can still complete G4; new
	//     enrollments in secure-go are rejected (rule 2)
	//   - retire allowed, live enrollments are closed (unenrolled_at set)
	//   - retire rejected while live incomplete enrollments exist
	t.Skip("TODO(you): assert the S13 outcome you choose")
}

// S14, unenroll Gina after she completed secure-go.
func TestS14_UnenrollAfterCompletion(t *testing.T) {
	// Arrange
	sut, ctx := newSUT(t)
	gina, _ := arrangeHanaS7(t, sut, ctx)

	// Act
	err := sut.Unenroll(ctx, web.UnenrollRequest{LearnerID: web.LearnerID(gina.LearnerID), PathKey: web.PathKey(gina.PathKey), UnenrolledAt: "2026-03-20T08:00:00Z"})

	// Assert
	if err != nil {
		t.Fatalf("unenroll: %v", err)
	}
	got := mustGet(t, sut, ctx, gina.ID)
	if got.UnenrolledAt != "2026-03-20T08:00:00Z" {
		t.Errorf("unenrolled_at = %q, want 2026-03-20T08:00:00Z", got.UnenrolledAt)
	}
	assertCompletedAt(t, got, "2026-01-22T10:00:00Z") // rule 6: completed_at is final
	if list, _ := sut.ListLearnerEnrollments(ctx, "gina"); len(list) != 1 {
		t.Errorf("history is never deleted: want gina's enrollment listed, got %d", len(list))
	}
}
