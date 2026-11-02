package enrollments

type EnrollmentID string

type Enrollment struct {
	ID           EnrollmentID `json:"id"`
	LearnerId    LearnerID    `json:"learner_id"`
	PathKey      PathKey      `json:"path_key"`
	Source       string       `json:"source"`
	AssignmentID *string      `json:"assignment_id"`
	CreditDays   *uint        `json:"credit_days"`
	EnrolledAt   DateString   `json:"enrolled_at"`
	CompletedAt  DateString   `json:"completed_at"`
	UnenrolledAt DateString   `json:"unenrolled_at"`
}
