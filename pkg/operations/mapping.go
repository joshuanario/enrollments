package operations

// Conversions between the domain types (pkg/enrollments), the database rows
// (pkg/store) and the UI views (pkg/web). Pure field mapping: no defaults,
// no validation beyond what a conversion itself needs.

import (
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/joshuanario/enrollments/pkg/enrollments"
	"github.com/joshuanario/enrollments/pkg/store"
	"github.com/joshuanario/enrollments/pkg/web"
)

// ---- shared ----

func statusToText(s enrollments.PathStatus) (string, error) {
	switch s {
	case enrollments.Active:
		return "active", nil
	case enrollments.Retired:
		return "retired", nil
	}
	return "", fmt.Errorf("unknown path status %d", s)
}

func statusFromText(s string) enrollments.PathStatus {
	switch s {
	case "active":
		return enrollments.Active
	case "retired":
		return enrollments.Retired
	}
	return enrollments.Undefined
}

// parseDate converts an ISO 8601 / RFC 3339 DateString to a UTC time.
// Unlike DateString.Time, an invalid value is an error, not "now".
func parseDate(d enrollments.DateString) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, string(d))
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid timestamp %q: %w", d, err)
	}
	return t.UTC(), nil
}

// formatDate renders a time as an RFC 3339 UTC DateString.
func formatDate(t time.Time) enrollments.DateString {
	return enrollments.DateString(t.UTC().Format(time.RFC3339))
}

// parseOptionalDate maps "" to nil.
func parseOptionalDate(d enrollments.DateString) (*time.Time, error) {
	if d == "" {
		return nil, nil
	}
	t, err := parseDate(d)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// formatOptionalDate maps nil to "".
func formatOptionalDate(t *time.Time) enrollments.DateString {
	if t == nil {
		return ""
	}
	return formatDate(*t)
}

func parseEnrollmentID(id enrollments.EnrollmentID) (int64, error) {
	n, err := strconv.ParseInt(string(id), 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid enrollment id %q", id)
	}
	return n, nil
}

func formatEnrollmentID(id int64) enrollments.EnrollmentID {
	return enrollments.EnrollmentID(strconv.FormatInt(id, 10))
}

func cloneString(p *string) *string {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

// ---- paths ----

func pathToRecord(p enrollments.Path) (store.PathRecord, error) {
	status, err := statusToText(p.Status)
	if err != nil {
		return store.PathRecord{}, err
	}
	lessons := make([]store.PathLessonRow, len(p.Lessons))
	for i, l := range p.Lessons {
		lessons[i] = store.PathLessonRow{PathKey: string(p.Key), Position: i, LessonKey: string(l)}
	}
	return store.PathRecord{
		Path:    store.PathRow{Key: string(p.Key), Name: p.Name, Status: status},
		Lessons: lessons,
	}, nil
}

// pathFromRecord expects rec.Lessons ordered by position.
func pathFromRecord(rec store.PathRecord) enrollments.Path {
	keys := make([]enrollments.LessonKey, len(rec.Lessons))
	for i, l := range rec.Lessons {
		keys[i] = enrollments.LessonKey(l.LessonKey)
	}
	return enrollments.Path{
		Key:     enrollments.PathKey(rec.Path.Key),
		Name:    rec.Path.Name,
		Lessons: keys,
		Status:  statusFromText(rec.Path.Status),
	}
}

// ---- enrollments ----

// enrollmentToRow maps an enrollment to its row. An empty ID maps to 0 so
// CreateEnrollment assigns one. EnrolledAt is required.
func enrollmentToRow(e enrollments.Enrollment) (store.EnrollmentRow, error) {
	var row store.EnrollmentRow
	if e.ID != "" {
		id, err := parseEnrollmentID(e.ID)
		if err != nil {
			return row, err
		}
		row.ID = id
	}
	enrolled, err := parseDate(e.EnrolledAt)
	if err != nil {
		return row, fmt.Errorf("enrolled_at: %w", err)
	}
	completed, err := parseOptionalDate(e.CompletedAt)
	if err != nil {
		return row, fmt.Errorf("completed_at: %w", err)
	}
	unenrolled, err := parseOptionalDate(e.UnenrolledAt)
	if err != nil {
		return row, fmt.Errorf("unenrolled_at: %w", err)
	}
	if e.CreditDays != nil {
		if *e.CreditDays > math.MaxInt32 {
			return row, fmt.Errorf("credit_days %d out of range", *e.CreditDays)
		}
		n := int(*e.CreditDays)
		row.CreditDays = &n
	}
	row.LearnerID = string(e.LearnerId)
	row.PathKey = string(e.PathKey)
	row.Source = e.Source
	row.AssignmentID = cloneString(e.AssignmentID)
	row.EnrolledAt = enrolled
	row.CompletedAt = completed
	row.UnenrolledAt = unenrolled
	return row, nil
}

func enrollmentFromRow(r store.EnrollmentRow) enrollments.Enrollment {
	e := enrollments.Enrollment{
		ID:           formatEnrollmentID(r.ID),
		LearnerId:    enrollments.LearnerID(r.LearnerID),
		PathKey:      enrollments.PathKey(r.PathKey),
		Source:       r.Source,
		AssignmentID: cloneString(r.AssignmentID),
		EnrolledAt:   formatDate(r.EnrolledAt),
		CompletedAt:  formatOptionalDate(r.CompletedAt),
		UnenrolledAt: formatOptionalDate(r.UnenrolledAt),
	}
	if r.CreditDays != nil && *r.CreditDays >= 0 {
		n := uint(*r.CreditDays)
		e.CreditDays = &n
	}
	return e
}

// enrollmentFromRequest maps the UI's enroll form to an enrollment with no ID.
// Fields are copied as given; defaults (e.g. EnrolledAt = now) are the
// service's job.
func enrollmentFromRequest(req web.EnrollRequest) (enrollments.Enrollment, error) {
	e := enrollments.Enrollment{
		LearnerId:  req.LearnerID,
		PathKey:    req.PathKey,
		Source:     req.Source,
		EnrolledAt: req.EnrolledAt,
	}
	if req.AssignmentID != "" {
		id := req.AssignmentID
		e.AssignmentID = &id
	}
	if req.CreditDays != nil {
		if *req.CreditDays < 0 {
			return e, fmt.Errorf("credit_days must not be negative, got %d", *req.CreditDays)
		}
		n := uint(*req.CreditDays)
		e.CreditDays = &n
	}
	return e, nil
}

// enrollmentToView maps an enrollment plus its already-computed lesson
// progress to what the UI renders.
func enrollmentToView(e enrollments.Enrollment, lessons []web.LessonProgressView) web.EnrollmentView {
	v := web.EnrollmentView{
		ID:           string(e.ID),
		LearnerID:    string(e.LearnerId),
		PathKey:      string(e.PathKey),
		Source:       e.Source,
		EnrolledAt:   string(e.EnrolledAt),
		CompletedAt:  string(e.CompletedAt),
		UnenrolledAt: string(e.UnenrolledAt),
		Lessons:      lessons,
	}
	if e.AssignmentID != nil {
		v.AssignmentID = *e.AssignmentID
	}
	if e.CreditDays != nil {
		n := int(*e.CreditDays)
		v.CreditDays = &n
	}
	if v.Lessons == nil {
		v.Lessons = []web.LessonProgressView{}
	}
	return v
}

// ---- activity events ----

func eventToRow(e enrollments.LessonEvent) (store.EventRow, error) {
	occurred, err := parseDate(e.OccurredAt)
	if err != nil {
		return store.EventRow{}, err
	}
	// TODO(you): LearnerId is dropped here because EventRow has no learner
	// column yet.
	return store.EventRow{
		EventID:    string(e.EventID),
		LessonKey:  string(e.LessonKey),
		Type:       e.Type,
		OccurredAt: occurred,
	}, nil
}

func eventFromRow(r store.EventRow) enrollments.LessonEvent {
	return enrollments.LessonEvent{
		EventID:    enrollments.EventID(r.EventID),
		LessonKey:  enrollments.LessonKey(r.LessonKey),
		Type:       r.Type,
		OccurredAt: formatDate(r.OccurredAt),
	}
}

func eventsFromRows(rows []store.EventRow) []enrollments.LessonEvent {
	out := make([]enrollments.LessonEvent, len(rows))
	for i, r := range rows {
		out[i] = eventFromRow(r)
	}
	return out
}
