package web

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gorilla/mux"
)

//go:embed templates/*.html
var templateFS embed.FS

var index = template.Must(template.ParseFS(templateFS, "templates/index.html"))

// Register mounts the single-page UI at / and the JSON endpoints it calls
// under /ui. The /ui endpoints only translate HTTP <-> Service; they are
// separate from the service's own public API.
//
// Fixture files (events.json, scenarios.json, paths.json) for the scenario
// player are served from $FIXTURES_DIR, default "test/fixtures".
//
// /ui/ws is a websocket that keeps the page's views live: the page sends
// {"learner": id} and gets that learner's enrollments now and after every
// /ui call that may have changed them.
func Register(r *mux.Router, svc Service) {
	h := handlers{svc: svc, live: newHub(svc)}
	fixtures := os.Getenv("FIXTURES_DIR")
	if fixtures == "" {
		fixtures = "test/fixtures"
	}

	r.HandleFunc("/", h.index).Methods("GET")

	ui := r.PathPrefix("/ui").Subrouter()
	ui.HandleFunc("/paths", h.listPaths).Methods("GET")
	ui.HandleFunc("/paths/{key}", h.upsertPath).Methods("POST")
	ui.HandleFunc("/learners/{learner}/enrollments", h.listEnrollments).Methods("GET")
	ui.HandleFunc("/learners/{learner}/enrollments", h.enroll).Methods("POST")
	ui.HandleFunc("/enrollments/{id}", h.getEnrollment).Methods("GET")
	ui.HandleFunc("/enrollments/{id}/unenroll", h.unenroll).Methods("POST")
	ui.HandleFunc("/ingest", h.ingest).Methods("POST")
	ui.HandleFunc("/ws", h.live.serve).Methods("GET")
	ui.PathPrefix("/fixtures/").Handler(http.StripPrefix("/ui/fixtures/", fixtureServer(fixtures))).Methods("GET")
}

type handlers struct {
	svc  Service
	live *hub
}

func (h handlers) index(w http.ResponseWriter, r *http.Request) {
	var buf bytes.Buffer
	if err := index.Execute(&buf, nil); err != nil {
		log.Printf("render index: %v", err)
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	buf.WriteTo(w)
}

// fixtureServer serves only *.json files from dir, without directory listings.
func fixtureServer(dir string) http.Handler {
	files := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, ".json") || strings.Contains(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		files.ServeHTTP(w, r)
	})
}

// ---- JSON helpers ----

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("encode response: %v", err)
	}
}

// writeErr reports a Service error. Service errors are shown to the user
// verbatim (e.g. the S9 rejections), so the status code is advisory only.
func writeErr(w http.ResponseWriter, err error) {
	status := http.StatusUnprocessableEntity
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func decodeBody(r *http.Request, v any) error {
	defer r.Body.Close()
	if err := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20)).Decode(v); err != nil {
		return fmt.Errorf("invalid JSON body: %w", err)
	}
	return nil
}

// ---- paths ----

type pathJSON struct {
	Key     string   `json:"key"`
	Name    string   `json:"name"`
	Lessons []string `json:"lessons"`
	Status  string   `json:"status"`
}

func (h handlers) listPaths(w http.ResponseWriter, r *http.Request) {
	paths, err := h.svc.ListPaths(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	out := make([]pathJSON, len(paths))
	for i, p := range paths {
		out[i] = pathJSON{Key: string(p.Key), Name: p.Name, Lessons: make([]string, len(p.Lessons)), Status: string(p.Status)}
		for j, l := range p.Lessons {
			out[i].Lessons[j] = string(l)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (h handlers) upsertPath(w http.ResponseWriter, r *http.Request) {
	var in pathJSON
	if err := decodeBody(r, &in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	p := Path{Key: PathKey(mux.Vars(r)["key"]), Name: in.Name}
	for _, l := range in.Lessons {
		if l = strings.TrimSpace(l); l != "" {
			p.Lessons = append(p.Lessons, LessonKey(l))
		}
	}
	p.Status = PathStatus(in.Status)
	if err := h.svc.UpsertPath(r.Context(), p); err != nil {
		writeErr(w, err)
		return
	}
	h.live.changedAll() // a path change can change any learner's progress
	w.WriteHeader(http.StatusNoContent)
}

// ---- enrollments ----

func (h handlers) listEnrollments(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.ListLearnerEnrollments(r.Context(), LearnerID(mux.Vars(r)["learner"]))
	if err != nil {
		writeErr(w, err)
		return
	}
	if list == nil {
		list = []EnrollmentView{}
	}
	writeJSON(w, http.StatusOK, list)
}

type enrollJSON struct {
	PathKey      string `json:"path_key"`
	Source       string `json:"source"`
	AssignmentID string `json:"assignment_id"`
	CreditDays   *int   `json:"credit_days"`
	EnrolledAt   string `json:"enrolled_at"`
}

func (h handlers) enroll(w http.ResponseWriter, r *http.Request) {
	var in enrollJSON
	if err := decodeBody(r, &in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	e, err := h.svc.Enroll(r.Context(), EnrollRequest{
		LearnerID:    LearnerID(mux.Vars(r)["learner"]),
		PathKey:      PathKey(in.PathKey),
		Source:       in.Source,
		AssignmentID: strings.TrimSpace(in.AssignmentID),
		CreditDays:   in.CreditDays,
		EnrolledAt:   DateString(strings.TrimSpace(in.EnrolledAt)),
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	h.live.changed(e.LearnerID)
	writeJSON(w, http.StatusCreated, e)
}

func (h handlers) getEnrollment(w http.ResponseWriter, r *http.Request) {
	e, err := h.svc.GetEnrollment(r.Context(), mux.Vars(r)["id"])
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, e)
}

func (h handlers) unenroll(w http.ResponseWriter, r *http.Request) {
	var in struct {
		UnenrolledAt string `json:"unenrolled_at"`
	}
	if err := decodeBody(r, &in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	// The URL names the enrollment; Unenroll wants its learner and path.
	e, err := h.svc.GetEnrollment(r.Context(), mux.Vars(r)["id"])
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := h.svc.Unenroll(r.Context(), UnenrollRequest{
		LearnerID:    LearnerID(e.LearnerID),
		PathKey:      PathKey(e.PathKey),
		UnenrolledAt: DateString(strings.TrimSpace(in.UnenrolledAt)),
	}); err != nil {
		writeErr(w, err)
		return
	}
	h.live.changed(e.LearnerID)
	h.getEnrollment(w, r)
}

// ---- ingest ----

// ingest accepts a single event object or an array of events. Field values
// are passed through untouched so the service decides what is invalid.
func (h handlers) ingest(w http.ResponseWriter, r *http.Request) {
	var raw json.RawMessage
	if err := decodeBody(r, &raw); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	events, err := decodeEvents(raw)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	results, err := h.svc.Ingest(r.Context(), events)
	learners := make([]string, len(events))
	for i, e := range events {
		learners[i] = string(e.LearnerID)
	}
	h.live.changed(learners...) // also on error: earlier events may be stored
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, results)
}

func decodeEvents(raw json.RawMessage) ([]LessonEvent, error) {
	if trimmed := bytes.TrimSpace(raw); len(trimmed) > 0 && trimmed[0] == '[' {
		var evs []LessonEvent
		if err := json.Unmarshal(trimmed, &evs); err != nil {
			return nil, fmt.Errorf("invalid events: %w", err)
		}
		return evs, nil
	}
	var ev LessonEvent
	if err := json.Unmarshal(raw, &ev); err != nil {
		return nil, fmt.Errorf("invalid event: %w", err)
	}
	return []LessonEvent{ev}, nil
}
