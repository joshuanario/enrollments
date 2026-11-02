// Package web serves the server-rendered HTML UI. It only renders and parses
// forms; all behavior goes through Service.
package web

import (
	"context"
	"errors"

	"github.com/joshuanario/enrollments/pkg/enrollments"
)

// Service is the boundary the UI calls.
type Service interface {
	UpsertPath(ctx context.Context, p enrollments.Path) error
	ListPaths(ctx context.Context) ([]enrollments.Path, error)
	Enroll(ctx context.Context, req EnrollRequest) (EnrollmentView, error)
	// Unenroll with at == "" means now.
	Unenroll(ctx context.Context, id string, at enrollments.DateString) error
	GetEnrollment(ctx context.Context, id string) (EnrollmentView, error)
	ListLearnerEnrollments(ctx context.Context, learner enrollments.LearnerID) ([]EnrollmentView, error)
	Ingest(ctx context.Context, events []enrollments.LessonEvent) ([]IngestResultView, error)
}

// EnrollRequest is what the enroll form submits. Empty EnrolledAt means now.
type EnrollRequest struct {
	LearnerID    enrollments.LearnerID
	PathKey      enrollments.PathKey
	Source       string // "self" | "assigned"
	AssignmentID string
	CreditDays   *int
	EnrolledAt   enrollments.DateString
}

// EnrollmentView is what the UI renders. Timestamps are RFC 3339 UTC
// strings; empty means null.
type EnrollmentView struct {
	ID           string               `json:"id"`
	LearnerID    string               `json:"learner_id"`
	PathKey      string               `json:"path_key"`
	Source       string               `json:"source"`
	AssignmentID string               `json:"assignment_id,omitempty"`
	CreditDays   *int                 `json:"credit_days"`
	EnrolledAt   string               `json:"enrolled_at"`
	CompletedAt  string               `json:"completed_at,omitempty"`
	UnenrolledAt string               `json:"unenrolled_at,omitempty"`
	Lessons      []LessonProgressView `json:"lessons"`
}

type LessonProgressView struct {
	LessonKey   string `json:"lesson_key"`
	StartedAt   string `json:"started_at,omitempty"`
	CompletedAt string `json:"completed_at,omitempty"`
	Credited    bool   `json:"credited"`
}

// IngestResultView reports what happened to one submitted event.
type IngestResultView struct {
	EventID  string `json:"event_id"`
	Accepted bool   `json:"accepted"`
	Reason   string `json:"reason,omitempty"` // e.g. "duplicate", "invalid type"
}

var ErrNotImplemented = errors.New("not implemented yet") //TODO rm this
