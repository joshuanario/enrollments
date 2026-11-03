# CLAUDE.md

A learning-path enrollment service in Go, written as a job-interview take-home. See ASSIGNMENT.md for the domain, rules 1–8, the API and scenarios S1–S14.

## Who owns what (read first)

The author has to defend the design in a follow-up interview, so most of the code must stay their own work.

- **The author owns:**
  - the domain model;
  - the business rules (rules 1–8);
  - event idempotency and dedupe (`pkg/operations/idempotencykey.go`);
  - pub-sub, the JSON API, README.md and NOTES.md;
  - the methods in `pkg/operations/service.go`, including `UpsertPath`, `Enroll`, `Unenroll` and the lesson sort functions.
- **Claude may work on:**
  - the Postgres/GORM store (`pkg/store`);
  - docker compose and the startup seed (`cmd/enrollments/seed.go`);
  - the web UI (`pkg/web`: handlers, the websocket hub, `templates/index.html`).
- **Service code Claude wrote because the author asked for it explicitly:**
  - `Ingest` (`ingest.go`);
  - progress computation (`progress.go`);
  - `reevaluatePathEnrollments`;
  - `ListPaths`;
  - the adjacency-list sort.
- **Outside that scope, diagnose and suggest, don't edit.** Take on another service method only when the author asks for that method by name. Leave `// TODO(you)` comments at the boundary.
- Don't change test code (`test/`) unless asked.
- Don't add database constraints that encode business rules (e.g. a "one live enrollment" unique index).

## Commands

```sh
go build ./... && go vet ./...
go test ./...                    # in-memory SQLite, no Docker needed
docker compose up -d --build     # app on :8080, Postgres on :5432
docker compose down -v           # reset the dev database (deletes all data)
```

- **Store tests can run against Postgres:** setting `DATABASE_URL` points them at a real database, but they **truncate every table**. Never point them at the dev database without asking.
- **`-sorting` flag:** chooses the lesson ordering strategy. `0` is fractional indexing (the default), `1` is the integer position column and `2` is the adjacency list.
- **Other strategies in tests:** `test/helpers_test.go` (`newSUT`) hardcodes `FractionalIndexing`. To run the scenarios under another strategy, swap it temporarily and restore the file byte for byte afterwards.

## Layout

- `cmd/enrollments/`
  - `main.go`: wiring and flags.
  - `seed.go`: when `SEED_SETUP=true` (set in compose), creates the `setup` paths from `test/fixtures` and skips paths that already exist.
- `pkg/store/`
  - `models.go`: GORM row models.
  - `repo.go`: plain CRUD, no business rules. Returns `ErrNotFound`.
  - `db.go`: Postgres or in-memory SQLite, and AutoMigrate.
- `pkg/operations/`: the service that implements `web.Service`.
  - `progress.go`: computes per-lesson progress from events when an enrollment is read.
  - `fractionalindexing.go`, `integerpositioncolumn.go`, `adjacencylist.go`: the lesson ordering strategies.
- `pkg/web/`
  - `types.go`, `service.go`: domain and view types, and the `Service` interface.
  - `handlers.go`: the `/ui/*` JSON endpoints.
  - `live.go`: the websocket hub.
  - `templates/index.html`: the single-page UI.
- `test/`: scenario tests S1–S14 against in-memory SQLite. `fixtures/` holds `events.json`, `paths.json` and `scenarios.json`.

## How the design works

- **Progress comes from events.** Events (`activity_events`) are the source of truth. Lesson progress is computed when an enrollment is read: rules 4 and 5, with the credit window cutoff set by `credit_days` on assigned enrollments.
- **Completion is final.** `completed_at` is stored and never changes once set (rule 6). Completion is re-checked in `Enroll`, `Ingest` (`reevaluateEnrollments`) and `UpsertPath` (`reevaluatePathEnrollments`).
- **Lesson rows.** A path's own lessons are the rows in `lessons` with an empty `learner_id`. A lesson dropped from a path isn't deleted: its row gets `status = "removed"`, and gets `""` again if the lesson comes back. `pathLessonKeys` gives the current lessons in order, with removed ones left out.
- **Ordering contract.** Each `SortLessonsAs*` function returns:
  - `sorted`: the rows in their stored order;
  - `updated`: the rows whose ordering columns must change for the stored order to match the input order.

  `UpsertPath` saves `updated` with `store.UpdateLessonRows`, which writes `status`, `next_id`, `order` and `index`.
- **Duplicate events.** A repeated `event_id` is a duplicate and the first write wins. An invalid event is rejected and not stored, so a corrected copy can be sent later.

## UI

- `/` serves a single-page app with two parts:
  - **Enrollment Viewer:** read-only. It shows a learner's enrollments and per-lesson progress, with a dropdown to switch learners.
  - **Scenario Player:** plays one playlist per scenario from `scenarios.json`. Tracks are ingest events, enroll, unenroll and upsert_path. Note and expect steps are skipped.
- `/ui/ws` keeps the views live. The page subscribes to a learner. The server pushes that learner's enrollments after any `/ui` call that may have changed them; a path upsert pushes to everyone. Changes made outside `/ui` don't trigger a push.
- Keep the UI matching `enrollments-wireframe.pdf`.

## Known gaps

- `SortLessonsAsFractionalIndexing`: when lessons are added at the end, `(low + high) / 2` overflows to `+Inf`. Its `continue` branch also doesn't move `low` forward. Integer and adjacency-list ordering work.
- The scenario tests check which lessons a path has, not their order, so ordering bugs don't fail tests.
- S11–S13 still have `TODO(you)` assertions.
