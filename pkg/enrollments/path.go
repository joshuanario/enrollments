package enrollments

type PathKey string

type PathStatus int

const (
	Undefined = iota
	Active
	Retired
)

type Path struct {
	Key     PathKey
	Name    string
	Lessons []LessonKey
	Status  PathStatus
}
