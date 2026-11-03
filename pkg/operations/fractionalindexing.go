package operations

import (
	"math"
	"slices"

	"github.com/joshuanario/enrollments/pkg/store"
	"github.com/joshuanario/enrollments/pkg/web"
)

const (
	minIndex = -math.MaxFloat64
	maxIndex = math.MaxFloat64
)

// SortLessonsAsFractionalIndexing sorts the lessons using fractional indexing and returns the lessons with updated indices.
// It ensures that each lesson has a unique index and that the indices are in ascending order.
func SortLessonsAsFractionalIndexing(lessons []store.LessonRow) (sorted, updated []store.LessonRow) {
	low := minIndex
	high := maxIndex
	if len(lessons) == 0 {
		return nil, nil
	}
	if verifyOrderWithFractionalIndexing(lessons) {
		return lessons, []store.LessonRow{} //nothing to update
	}
	updatedLessons := make([]store.LessonRow, 0)
	if len(lessons) == 1 {
		lesson := lessons[0]
		lesson.Index = &low
		updatedLessons = append(updatedLessons, lesson)
		return lessons, updatedLessons
	}
	if len(lessons) == 2 {
		lesson1 := lessons[0]
		lesson2 := lessons[1]
		lesson1.Index = &low
		lesson2.Index = &high
		updatedLessons = append(updatedLessons, lesson1, lesson2)
		return lessons, updatedLessons
	}
	for i := 0; i < len(lessons)-1; i++ {
		curr := lessons[i]
		next := lessons[i+1]
		if curr.Index != nil && next.Index != nil && *curr.Index < *next.Index {
			continue
		} else {
			mid := (low + high) / 2
			curr.Index = &mid
			low = mid
		}
		updatedLessons = append(updatedLessons, curr)
		if i == len(lessons)-2 {
			if next.Index == nil || *next.Index != high {
				next.Index = &high
				updatedLessons = append(updatedLessons, next)
			}
		}
	}

	slices.SortFunc(lessons, func(a, b store.LessonRow) int {
		if a.Index == nil && b.Index == nil {
			return 0
		}
		if a.Index == nil {
			return -1
		}
		if b.Index == nil {
			return 1
		}
		if *a.Index < *b.Index {
			return -1
		}
		if *a.Index > *b.Index {
			return 1
		}
		return 0
	})
	return lessons, updatedLessons
}

func GetLessonProgressViewAsFractionalIndexing(path store.PathRecord) []web.LessonProgressView {
	path.Lessons, _ = SortLessonsAsFractionalIndexing(path.Lessons)
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

func verifyOrderWithFractionalIndexing(lessons []store.LessonRow) bool {
	for i := 0; i < len(lessons)-1; i++ {
		if lessons[i].Index == nil || lessons[i+1].Index == nil {
			return false
		}
		if *lessons[i].Index >= *lessons[i+1].Index {
			return false
		}
	}
	return true
}
