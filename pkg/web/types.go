package web

// Types the UI and Service share. Values use the same text forms the store
// keeps, so no separate mapping layer is needed.

type (
	PathKey      string
	LessonKey    string
	LearnerID    string
	EventID      string
	EnrollmentID string
)

// DateString is an ISO 8601 / RFC 3339 timestamp, e.g. "2026-03-01T08:00:00Z".
type DateString string

// PathStatus is stored as-is in paths.status.
type PathStatus string

const (
	PathActive  PathStatus = "active"
	PathRetired PathStatus = "retired"
)

type Path struct {
	Key     PathKey     `json:"key"`
	Name    string      `json:"name"`
	Lessons []LessonKey `json:"lessons"`
	Status  PathStatus  `json:"status"`
}

type LessonEvent struct {
	EventID    EventID    `json:"event_id"`
	LearnerID  LearnerID  `json:"learner_id"`
	LessonKey  LessonKey  `json:"lesson_key"`
	Type       string     `json:"type"`
	OccurredAt DateString `json:"occurred_at"`
}
