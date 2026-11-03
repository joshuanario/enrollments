package operations

import (
	"github.com/joshuanario/enrollments/pkg/store"
	"github.com/joshuanario/enrollments/pkg/web"
)

// SortLessonsAsAdjacencyList returns the lessons in their stored NextId order
// (sorted) and the rows whose NextId must change so that the stored links
// follow the input order instead (updated): each lesson points to the one after
// it and the last lesson has a nil NextId. Lessons with status "removed" are
// not linked and come last in sorted.
func SortLessonsAsAdjacencyList(lessons []store.LessonRow) (sorted []store.LessonRow, updated []store.LessonRow) {
	if len(lessons) == 0 {
		return nil, nil
	}
	live := make([]store.LessonRow, 0, len(lessons))
	removed := make([]store.LessonRow, 0)
	for _, l := range lessons {
		if l.Status == "removed" {
			removed = append(removed, l)
		} else {
			live = append(live, l)
		}
	}

	updatedLessons := make([]store.LessonRow, 0)
	for i, curr := range live {
		var want *int64
		if i < len(live)-1 {
			id := live[i+1].ID
			want = &id
		}
		if !sameNextId(curr.NextId, want) {
			curr.NextId = want
			updatedLessons = append(updatedLessons, curr)
		}
	}

	sorted = append(walkNextIds(live), removed...)
	return sorted, updatedLessons
}

func GetLessonProgressViewAsAdjacencyList(path store.PathRecord) []web.LessonProgressView {
	path.Lessons, _ = SortLessonsAsAdjacencyList(path.Lessons)
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

func sameNextId(a, b *int64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// walkNextIds orders the lessons by following their stored NextId links from
// each head (a lesson no other lesson points to), in input order. Lessons the
// walk doesn't reach, e.g. in a cycle or before their links are written, are
// appended in input order, so no lesson is ever dropped.
func walkNextIds(lessons []store.LessonRow) []store.LessonRow {
	byId := make(map[int64]store.LessonRow, len(lessons))
	pointedTo := make(map[int64]bool, len(lessons))
	for _, l := range lessons {
		byId[l.ID] = l
	}
	for _, l := range lessons {
		if l.NextId != nil {
			if _, ok := byId[*l.NextId]; ok {
				pointedTo[*l.NextId] = true
			}
		}
	}

	sorted := make([]store.LessonRow, 0, len(lessons))
	visited := make(map[int64]bool, len(lessons))
	walk := func(start store.LessonRow) {
		for curr, ok := start, true; ok && !visited[curr.ID]; {
			visited[curr.ID] = true
			sorted = append(sorted, curr)
			if curr.NextId == nil {
				return
			}
			curr, ok = byId[*curr.NextId]
		}
	}
	for _, l := range lessons {
		if !pointedTo[l.ID] {
			walk(l)
		}
	}
	for _, l := range lessons {
		if !visited[l.ID] {
			walk(l)
		}
	}
	return sorted
}
