// Package web serves the server-rendered HTML UI. It only renders and parses
// forms; all behavior goes through Service.
package web

import (
	"context"
)

// Service is the boundary the UI calls.
type Service interface {
	UpsertPath(ctx context.Context, p Path) error
	ListPaths(ctx context.Context) ([]Path, error)
	Enroll(ctx context.Context, req EnrollRequest) (EnrollmentView, error)
	Unenroll(ctx context.Context, req UnenrollRequest) error
	GetEnrollment(ctx context.Context, id string) (EnrollmentView, error)
	ListLearnerEnrollments(ctx context.Context, learner LearnerID) ([]EnrollmentView, error)
	Ingest(ctx context.Context, events []LessonEvent) ([]IngestResultView, error)
}

// EnrollRequest is what the enroll form submits. Empty EnrolledAt means now.
type EnrollRequest struct {
	LearnerID    LearnerID
	PathKey      PathKey
	Source       string // "self" | "assigned"
	AssignmentID string
	CreditDays   *int
	EnrolledAt   DateString
}

// UnenrollRequest is what the unenroll form submits. Empty UnenrolledAt means now.
type UnenrollRequest struct {
	LearnerID    LearnerID
	PathKey      PathKey
	UnenrolledAt DateString
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
	EnrolledAt   DateString           `json:"enrolled_at"`
	CompletedAt  DateString           `json:"completed_at,omitempty"`
	UnenrolledAt DateString           `json:"unenrolled_at,omitempty"`
	Lessons      []LessonProgressView `json:"lessons"`
}

type LessonProgressView struct {
	LessonKey   string     `json:"lesson_key"`
	StartedAt   DateString `json:"started_at,omitempty"`
	CompletedAt DateString `json:"completed_at,omitempty"`
	Credited    bool       `json:"credited"`
}

// IngestResultView reports what happened to one submitted event.
type IngestResultView struct {
	EventID  string `json:"event_id"`
	Accepted bool   `json:"accepted"`
	Reason   string `json:"reason,omitempty"` // e.g. "duplicate", "invalid type"
}
