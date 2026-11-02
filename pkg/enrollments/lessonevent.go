package enrollments

type EventID string
type LearnerID string
type LessonKey string

type LessonEvent struct {
	EventID    EventID    `json:"event_id"`
	LearnerId  LearnerID  `json:"learner_id"`
	LessonKey  LessonKey  `json:"lesson_key"`
	Type       string     `json:"type"`
	OccurredAt DateString `json:"occurred_at"`
}
