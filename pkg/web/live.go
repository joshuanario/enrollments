package web

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// hub keeps the UI's views live. Each websocket client subscribes to one
// learner and is sent that learner's enrollments on subscribe and whenever a
// /ui call may have changed them. It only sees changes made through /ui.
type hub struct {
	svc     Service
	mu      sync.Mutex
	clients map[*liveClient]struct{}
}

type liveClient struct {
	mu      sync.Mutex
	learner string
	// dirty is a one-slot signal: several changes before the next send
	// collapse into one reload.
	dirty chan struct{}
}

// clientMsg is what the page sends: the learner to watch.
type clientMsg struct {
	Learner string `json:"learner"`
}

// serverMsg is what the page receives.
type serverMsg struct {
	Type        string           `json:"type"` // "enrollments" | "error"
	Learner     string           `json:"learner"`
	Enrollments []EnrollmentView `json:"enrollments"`
	Error       string           `json:"error,omitempty"`
}

var upgrader = websocket.Upgrader{ReadBufferSize: 1024, WriteBufferSize: 1 << 16}

func newHub(svc Service) *hub {
	return &hub{svc: svc, clients: make(map[*liveClient]struct{})}
}

// changed tells clients watching any of the learners to reload.
func (h *hub) changed(learners ...string) {
	set := make(map[string]bool, len(learners))
	for _, l := range learners {
		set[l] = true
	}
	h.signal(func(l string) bool { return set[l] })
}

// changedAll tells every client to reload, e.g. after a path change.
func (h *hub) changedAll() {
	h.signal(func(string) bool { return true })
}

func (h *hub) signal(match func(learner string) bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		c.mu.Lock()
		l := c.learner
		c.mu.Unlock()
		if l != "" && match(l) {
			c.poke()
		}
	}
}

func (c *liveClient) poke() {
	select {
	case c.dirty <- struct{}{}:
	default:
	}
}

func (h *hub) serve(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return // Upgrade already wrote the error response
	}
	defer conn.Close()

	c := &liveClient{dirty: make(chan struct{}, 1)}
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.clients, c)
		h.mu.Unlock()
	}()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	go func() {
		defer cancel()
		for {
			var m clientMsg
			if err := conn.ReadJSON(&m); err != nil {
				return
			}
			c.mu.Lock()
			c.learner = m.Learner
			c.mu.Unlock()
			c.poke()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.dirty:
		}
		c.mu.Lock()
		learner := c.learner
		c.mu.Unlock()
		if learner == "" {
			continue
		}
		msg := serverMsg{Type: "enrollments", Learner: learner}
		list, err := h.svc.ListLearnerEnrollments(ctx, LearnerID(learner))
		if err != nil {
			msg = serverMsg{Type: "error", Learner: learner, Error: err.Error()}
		} else if list == nil {
			list = []EnrollmentView{}
		}
		msg.Enrollments = list
		b, err := json.Marshal(msg)
		if err != nil {
			log.Printf("live: encode: %v", err)
			continue
		}
		conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if err := conn.WriteMessage(websocket.TextMessage, b); err != nil {
			return
		}
	}
}
