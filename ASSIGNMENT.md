# Take-home: Learning Path Enrollment Service

Thank you for taking the time to do this. We have tried to make it a realistic slice of the kind of problem we solve, small enough to finish in an evening or two. Please do not spend more than about six hours. If you run out of time, stop, and tell us in your notes what you would have done next. We would rather see a well-reasoned partial solution than a rushed complete one.

## The problem

We run a learning platform. Learners work through **paths**, and a path is an ordered list of **lessons**. A learner can enroll in a path themselves, or an administrator can assign a path to them. Lesson activity (someone started or completed a lesson) arrives from a separate system as a stream of events.

Build a small service that owns enrollments and answers the question "how far along is this learner in this path?"

Constraints on the submission:

- Any language.
- Any storage. SQLite or an embedded store is fine.
- If you use Postgres, include a `docker-compose.yml`.
- The service and its tests must run with one or two documented commands.

## Domain

**Path**

| Field     | Notes                       |
| --------- | --------------------------- |
| `key`     | string, unique, stable      |
| `name`    | display name                |
| `lessons` | ordered list of lesson keys |
| `status`  | `active` or `retired`       |

**Enrollment**

| Field           | Notes                                                                    |
| --------------- | ------------------------------------------------------------------------ |
| `id`            | assigned by the service                                                  |
| `learner_id`    | string                                                                   |
| `path_key`      | string                                                                   |
| `source`        | `self` or `assigned`                                                     |
| `assignment_id` | string, present only when `source` is `assigned`                         |
| `credit_days`   | integer or null, meaningful only when `source` is `assigned`, see rule 4 |
| `enrolled_at`   | timestamp                                                                |
| `completed_at`  | timestamp or null                                                        |
| `unenrolled_at` | timestamp or null                                                        |

An enrollment is **live** when `unenrolled_at` is null.

**Lesson activity event**

```json
{
  "event_id": "evt-0001",
  "learner_id": "alice",
  "lesson_key": "L1",
  "type": "started",
  "occurred_at": "2026-03-04T10:00:00Z"
}
```

Properties of the stream:

- `type` is `started` or `completed`.
- `event_id` is unique per event.
- An event can be delivered more than once.
- Events can arrive out of order relative to `occurred_at`.

## Rules

1. **One live enrollment per learner and path.**
   - A second enroll while one is live is rejected.
   - After an unenroll, a new enroll is allowed and creates a new enrollment.
   - History is never deleted.

2. **Enrolling in a retired path is rejected.**

3. **Activity is idempotent and order-independent.**
   - Processing the same `event_id` twice has no additional effect.
   - The outcome must be the same regardless of the order in which events arrive.

4. **A lesson counts as complete for an enrollment when there is a `completed` event for that learner and lesson with `occurred_at` on or after the enrollment's cutoff.** The cutoff is an integer representing the number of days to search lesson completion history to apply to the assignment. Requirements:
   - `self` enrollments: no cutoff. Any completion ever counts. (Unlimited look-back.)
   - `assigned` with `credit_days` null: no cutoff, same as self.
   - `assigned` with `credit_days` 0: cutoff is `enrolled_at`. The learner must redo every lesson.
   - `assigned` with `credit_days` N: cutoff is `enrolled_at` minus N days.

   A lesson completed before `enrolled_at` but after the cutoff is **credited**: report it as complete and mark it credited.

5. **Lesson progress fields.** For each lesson in the enrollment's path, report:
   - `started_at`: the earliest `started` or `completed` event at or after the cutoff.
   - `completed_at`: the earliest `completed` event at or after the cutoff.
   - `credited`: true when `completed_at` is before `enrolled_at`.

6. **An enrollment is complete when every lesson in its path is complete.**
   - `completed_at` on the enrollment is the latest lesson `completed_at`.
   - When every lesson was credited, `completed_at` is `enrolled_at`.
   - Once set, `completed_at` is final. Later changes to the path do not reopen a completed enrollment.

7. **Paths can change after enrollments exist.** When a path's lesson list changes:
   - Live incomplete enrollments follow the new list.
   - A removed lesson no longer counts.
   - An added lesson is evaluated with rule 4, so prior completions inside the cutoff window still credit it.
   - A live enrollment can become complete because a lesson was removed.

8. **Activity for a lesson that is not in any of the learner's paths is still recorded.**
   - It may credit a future enrollment.

Rule 4 is written so that completion is a function of the learner's full activity history, the enrollment's `enrolled_at`, and `credit_days`.

Anything the rules do not say is deliberately unspecified. Use your judgment, write down what you decided, and be prepared to back up your decisions in the follow-up interview.

## API

Shape is up to you (REST or gRPC). The operations we need:

| Operation                    | Behavior                                                                                          |
| ---------------------------- | ------------------------------------------------------------------------------------------------- |
| Upsert path                  | Create or replace a path by key, including its lesson list and status.                            |
| Enroll                       | Create an enrollment. Rejects per rules 1 and 2. Returns the enrollment with its lesson progress. |
| Unenroll                     | Set `unenrolled_at`. Idempotent.                                                                  |
| Get enrollment               | The enrollment and per-lesson progress.                                                           |
| List a learner's enrollments | All of them, live and historical, with progress.                                                  |
| Ingest activity              | Accept one or many events. Must satisfy rule 3.                                                   |

Conventions:

- Timestamps are ISO 8601 in UTC.
- Enroll and unenroll requests may carry an explicit `enrolled_at` or `unenrolled_at` so the scenarios below are reproducible.
- Default those to now when absent.

## Fixtures and scenarios

`fixtures/paths.json` holds the paths, `fixtures/events.json` holds the activity events, and `fixtures/scenarios.json` is the ordered script, each tagged with the scenario it belongs to. Your tests should cover the scenarios below, in whatever form you prefer. You do not have to load the fixture files literally; they are there to save you typing.

**S1, credit on self enrollment.** Alice completed `L1` on 2025-11-15. She self-enrolls in `web-basics` (`L1`, `L2`, `L3`) on 2026-03-01. Expect `L1` complete and credited, 1 of 3 done.

**S2, duplicate delivery.** The `L1` completion event for Alice is delivered again with the same `event_id`. Nothing changes.

**S3, out-of-order delivery.** Alice's `L2` completed event (occurred 2026-03-05) arrives before her `L2` started event (occurred 2026-03-04). Expect `L2` complete with `started_at` 2026-03-04 and `completed_at` 2026-03-05.

**S4, credit window on assigned enrollments.** Bob, Carol, and Dave each completed `L1` on 2026-02-20 and are each assigned `web-basics` on 2026-03-01.

- Bob, `credit_days` 0: `L1` not complete.
- Carol, `credit_days` 30: `L1` complete and credited.
- Dave, `credit_days` null: `L1` complete and credited.
  Erin completed `L1` on 2024-06-01 and is assigned with `credit_days` 30: not complete.

**S5, enrollment completion.** Alice completes `L3` on 2026-03-10. With `L1` credited and `L2` done, her enrollment is complete with `completed_at` 2026-03-10.

**S6, lesson removed from a path.** Frank is live in `secure-go` (`G1`, `G2`, `G3`) with `G1` and `G2` complete. The path is replaced with (`G1`, `G2`). Frank's enrollment is now complete.

**S7, lesson added to a path.** After S6 the path is (`G1`, `G2`).

- Gina self-enrolled in January and completed both, so her enrollment has `completed_at` set.
- Hana self-enrolls on 2026-03-03.
- The path is replaced with (`G1`, `G2`, `G3`, `G4`).
- After that, Hana's completions of `G1`, `G2`, and `G3` arrive (2026-03-04 to 2026-03-06).
- Expect: Gina stays complete with her original `completed_at`. Hana is 3 of 4.
- Variant: if Hana also has a `G4` completion from 2025-05-05, she is 4 of 4 and complete, because self enrollments have unlimited look-back.

**S8, re-enroll after unenroll.** Alice unenrolls from `web-basics` on 2026-03-11 and self-enrolls again on 2026-03-12. Listing her enrollments shows two rows, the first still showing its 2026-03-10 completion. The new one credits all three lessons and is complete immediately, with `completed_at` equal to its `enrolled_at` (rule 6).

**S9, rejections.** Enrolling Alice in `web-basics` while her 2026-03-12 enrollment is live is rejected. Enrolling anyone in `legacy-path` (retired) is rejected.

**S10, orphan activity.** Ivan completes `Q7`, which is in no path. Later a path `quickstart` (`Q7`, `Q8`) is created and Ivan self-enrolls. `Q7` is credited.

**S11, repeated id, different payload.** Alice's `L2` completion (`evt-s3-01`) is delivered again with `occurred_at` 2026-03-07.

**S12, mixed batch.** A single ingest call carries three events for Ivan on `Q8`: a `started`, one with `type` `finished`, and a `completed` whose `occurred_at` is `not-a-timestamp`.

**S13, retire with learners in flight.** `secure-go` is retired while Hana is live and incomplete.

**S14, unenroll after completion.** Gina is unenrolled from `secure-go` on 2026-03-20.

## What to hand in

- A repository (zip or link) with the code and tests.
- A `README` with run and test instructions.
- `NOTES.md`, about a page or two, covering:
  - The data model and why.
  - How you handle rules 3, 6, and 7.
  - The decisions you made where the specification was silent, and why.
  - What you would change if events arrived from a message broker at high volume.
  - What you cut for time.

## What we look at

- Correctness against the scenarios.
- Clarity of the data model.
- How you handle idempotency and ordering.
- Test quality.
- The reasoning in your notes.

We do not grade on framework choice, UI, auth, or deployment. Do not build those.
