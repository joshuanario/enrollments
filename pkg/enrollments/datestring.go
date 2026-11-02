package enrollments

import "time"

type DateString string

type DateStamp struct {
	time time.Time
}

func (ds DateString) Time() time.Time {
	t, err := time.Parse(time.RFC3339, string(ds))
	if err != nil {
		return time.Now().UTC()
	}
	return t.UTC()
}

func NewDateStamp(ds DateString) DateStamp {
	t := ds.Time()
	return DateStamp{time: t}
}

func (ds DateStamp) String() string {
	return ds.time.Format(time.RFC3339)
}
