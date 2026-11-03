package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/joshuanario/enrollments/pkg/operations"
	"github.com/joshuanario/enrollments/pkg/store"
	"github.com/joshuanario/enrollments/pkg/web"
)

type fixtureStep struct {
	Scenario string   `json:"scenario"`
	Action   string   `json:"action"`
	Paths    []string `json:"paths"`
}

type fixturePath struct {
	Scenario string   `json:"scenario"`
	Key      string   `json:"key"`
	Name     string   `json:"name"`
	Lessons  []string `json:"lessons"`
	Status   string   `json:"status"`
}

// seedSetupPaths upserts the paths named by the "setup" upsert_path step in
// scenarios.json, using their "setup" entries in paths.json. Paths that
// already exist are left alone, so a restart doesn't undo later changes.
func seedSetupPaths(ctx context.Context, db *store.Store, svc operations.OperationsService, fixturesDir string) error {
	var scenarios struct {
		Steps []fixtureStep `json:"steps"`
	}
	if err := readJSON(filepath.Join(fixturesDir, "scenarios.json"), &scenarios); err != nil {
		return err
	}
	var paths []fixturePath
	if err := readJSON(filepath.Join(fixturesDir, "paths.json"), &paths); err != nil {
		return err
	}
	setup := make(map[string]fixturePath)
	for _, p := range paths {
		if p.Scenario == "setup" {
			setup[p.Key] = p
		}
	}

	for _, step := range scenarios.Steps {
		if step.Scenario != "setup" || step.Action != "upsert_path" {
			continue
		}
		for _, key := range step.Paths {
			p, ok := setup[key]
			if !ok {
				return fmt.Errorf("seed: setup path %q not in paths.json", key)
			}
			if _, err := db.GetPath(ctx, key); err == nil {
				log.Printf("seed: path %s already exists, skipping", key)
				continue
			} else if !errors.Is(err, store.ErrNotFound) {
				return err
			}
			wp := web.Path{Key: web.PathKey(p.Key), Name: p.Name, Status: web.PathStatus(p.Status)}
			for _, l := range p.Lessons {
				wp.Lessons = append(wp.Lessons, web.LessonKey(l))
			}
			if err := svc.UpsertPath(ctx, wp); err != nil {
				return fmt.Errorf("seed: upsert path %s: %w", key, err)
			}
			log.Printf("seed: upserted path %s %v", key, p.Lessons)
		}
	}
	return nil
}

func readJSON(name string, v any) error {
	b, err := os.ReadFile(name)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}
