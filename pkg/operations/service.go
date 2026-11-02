package operations

import (
	"context"

	"github.com/joshuanario/enrollments/pkg/enrollments"
	"github.com/joshuanario/enrollments/pkg/store"
	"github.com/joshuanario/enrollments/pkg/web"
)

// OperationsService lets the UI run before the real Service exists.
type OperationsService struct {
	store *store.Store
}

func NewOperationsService(s *store.Store) OperationsService {
	return OperationsService{store: s}
}

func (OperationsService) UpsertPath(context.Context, enrollments.Path) error {

	return nil
}
func (OperationsService) ListPaths(context.Context) ([]enrollments.Path, error) {
	return nil, nil
}
func (OperationsService) Enroll(context.Context, web.EnrollRequest) (web.EnrollmentView, error) {
	return web.EnrollmentView{}, nil
}
func (OperationsService) Unenroll(context.Context, string, enrollments.DateString) error {
	return nil
}
func (OperationsService) GetEnrollment(context.Context, string) (web.EnrollmentView, error) {
	return web.EnrollmentView{}, nil
}
func (OperationsService) ListLearnerEnrollments(context.Context, enrollments.LearnerID) ([]web.EnrollmentView, error) {
	return nil, nil
}
func (OperationsService) Ingest(context.Context, []enrollments.LessonEvent) ([]web.IngestResultView, error) {
	return nil, nil
}
