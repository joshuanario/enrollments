package operations

import (
	"sort"

	"github.com/joshuanario/enrollments/pkg/store"
	"github.com/joshuanario/enrollments/pkg/web"
)

// SortLessonsAsIntegerPositionColumn sorts the lessons using an integer position column and returns the lessons with updated Order pointers.
// It ensures that each lesson has a unique and ascending Order value.
func SortLessonsAsIntegerPositionColumn(lessons []store.LessonRow) (sorted, updated []store.LessonRow) {
	if len(lessons) == 0 {
		return nil, nil
	}
	if verifyOrderWithIntegerPositionColumn(lessons) {
		return lessons, []store.LessonRow{} //nothing to update
	}
	updatedLessons := make([]store.LessonRow, 0)
	for i := 0; i < len(lessons); i++ {
		lesson := lessons[i]
		uintI := uint(i)
		if lesson.Order != &uintI {
			lesson.Order = &uintI
			updatedLessons = append(updatedLessons, lesson)
		}
	}
	sort.Slice(lessons, func(i, j int) bool {
		if lessons[i].Order == nil && lessons[j].Order == nil {
			return false
		}
		if lessons[i].Order == nil {
			return true
		}
		if lessons[j].Order == nil {
			return false
		}
		return *lessons[i].Order < *lessons[j].Order
	})
	return lessons, updatedLessons
}

func GetLessonProgressViewAsIntegerPositionColumn(path store.PathRecord) []web.LessonProgressView {
	path.Lessons, _ = SortLessonsAsIntegerPositionColumn(path.Lessons)
	result := make([]web.LessonProgressView, 0)
	for _, lesson := range path.Lessons {
		result = append(result, web.LessonProgressView{
			LessonKey:   lesson.LessonKey,
			StartedAt:   toDateString(lesson.StartedAt),
			CompletedAt: toOptionalDateString(lesson.CompletedAt),
			Credited:    lesson.Credited,
		})
	}
	return result
}

func verifyOrderWithIntegerPositionColumn(lessons []store.LessonRow) bool {
	for i := 0; i < len(lessons)-1; i++ {
		if lessons[i].Order == nil || lessons[i+1].Order == nil {
			return false
		}
		if *lessons[i].Order >= *lessons[i+1].Order {
			return false
		}
	}
	return true
}
