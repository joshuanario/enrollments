package store

import "time"

// PathRow is one row per path. Its lessons are LessonRows with this PathKey.
type PathRow struct {
	Key       string `gorm:"primaryKey"`
	Name      string `gorm:"not null"`
	Status    string `gorm:"not null"` // "active" | "retired"
	UpdatedAt time.Time
}

func (PathRow) TableName() string { return "paths" }

// EnrollmentRow mirrors the Enrollment fields in ASSIGNMENT.md.
//
// TODO(you): rule 1 could also be backed by a partial unique index, e.g.
//
//	CREATE UNIQUE INDEX enrollments_one_live
//	  ON enrollments (learner_id, path_key) WHERE unenrolled_at IS NULL;
//
// Left out on purpose so enforcement stays in your service.
type EnrollmentRow struct {
	ID           int64  `gorm:"primaryKey"`
	LearnerID    string `gorm:"not null;index:idx_enrollments_learner_path"`
	PathKey      string `gorm:"not null;index:idx_enrollments_learner_path"`
	Source       string `gorm:"not null"` // "self" | "assigned"
	AssignmentID *string
	CreditDays   *int
	EnrolledAt   time.Time `gorm:"not null"`
	CompletedAt  *time.Time
	UnenrolledAt *time.Time
}

func (EnrollmentRow) TableName() string { return "enrollments" }

type LessonRow struct {
	ID        int64  `gorm:"primaryKey"`
	LearnerID string `gorm:"not null;uniqueIndex:idx_uq_key;index:idx_learner_lesson"`
	LessonKey string `gorm:"not null;uniqueIndex:idx_uq_key;index:idx_learner_lesson"`
	Status    string `gorm:"not null"`

	StartedAt   time.Time `gorm:"not null"`
	CompletedAt *time.Time
	Credited    bool `gorm:"not null;default:false"`

	PathKey string   `gorm:"not null;index"`
	Path    *PathRow `gorm:"foreignKey:PathKey;references:Key;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;"`

	// NextLesson/NextId are attributes used for adjacency list implementation of an ordered list
	NextId     *int64
	NextLesson *LessonRow `gorm:"foreignKey:NextId;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`

	//Order is used for integer position column implementation of an ordered list
	Order *uint

	// Index is used for fractional-indexing implementation of an ordered list
	Index *float64
}

func (LessonRow) TableName() string { return "lessons" }

type EventRow struct {
	ID         int64     `gorm:"primaryKey"`
	EventID    string    `gorm:"not null;uniqueIndex"`
	LearnerID  string    `gorm:"not null;index"`
	LessonKey  string    `gorm:"not null;index"`
	Type       string    `gorm:"not null"`
	OccurredAt time.Time `gorm:"not null"`
	ReceivedAt time.Time `gorm:"autoCreateTime"`
}

func (EventRow) TableName() string { return "activity_events" }
