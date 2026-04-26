# Receipt Parsing Backend Hardening Spec

## Scope

This spec covers the next backend slice for async receipt parsing:

- API and service tests for the current receipt parsing lifecycle.
- A local smoke scenario that can be run against a live backend.
- Durable async execution that survives backend process restarts.

Frontend integration and the real OpenAI parser adapter are out of scope for this slice.

## Current Problem

Receipt parsing jobs are persisted in `receipt_parse_jobs`, but the worker queue is an in-process channel and uploaded image bytes live only in memory. If the backend pod restarts, queued work is lost. If a job was already processing, it can stay in `processing` forever. A restarted worker also cannot recover the receipt image because `receipt_parse_files.storage_key` is not populated.

## Goals

- Keep the public receipt parsing HTTP contract stable.
- Persist uploaded receipt files before enqueueing work.
- Replace in-memory-only queue semantics with a database-backed polling worker.
- Recover stale `processing` jobs.
- Add tests for API lifecycle, service lifecycle, and approve atomicity.
- Add a local smoke script for the happy path.

## Non-Goals

- No object storage integration yet. Local filesystem storage is enough for dev/MVP.
- No multi-image upload UI.
- No OpenAI adapter in this slice.
- No distributed scheduler dependency.

## Data Model Changes

Extend `receipt_parse_jobs`:

- `attempt_count integer not null default 0`
- `last_attempt_at timestamptz null`
- `next_attempt_at timestamptz null`
- `locked_at timestamptz null`
- `locked_by text null`

Reuse `receipt_parse_files.storage_key` for persisted file location.

## Worker Design

The service starts a bounded polling worker loop.

Worker behavior:

- On startup, mark stale `processing` jobs as `queued`.
- Periodically acquire one due `queued` job by setting it to `processing`, incrementing `attempt_count`, and setting lock metadata.
- Load the first receipt file for the job from file storage.
- Parse through the existing `Parser` interface.
- Save items/draft expenses and mark the job `ready`.
- Mark failed jobs as `failed` with error metadata.
- Before saving parser results, re-read the job and skip persistence if it was cancelled.

The existing channel can remain only as a wake-up signal. Correctness must not depend on it.

## File Storage

Add a backend-only local receipt file store:

- Write uploaded file bytes under `data/receipt-parses/<job-id>/<file-id>`.
- Store the relative key in `receipt_parse_files.storage_key`.
- Read file bytes by `storage_key` when the worker claims a job.

For prod later, this interface can be replaced with S3/object storage.

## API Tests

Add HTTP handler tests for:

- `POST /receipt-parses` multipart happy path returns queued/accepted and stores a recoverable file.
- Creating a second parse while one active job exists returns `409 active_receipt_parse_exists`.
- `GET /receipt-parses/active` returns `{ "item": null }` when there is no active job and an item when one exists.
- `GET /receipt-parses/{id}` returns ready draft expenses and failed error payloads.
- approve/cancel map domain errors to the expected HTTP statuses.

## Service Tests

Add or keep tests for:

- create stores job and file metadata.
- worker can recover and process a queued job from database state.
- stale processing jobs are requeued.
- cancel prevents parser results from overwriting cancelled state.
- approve rolls back created expenses if receipt state update fails.

## Smoke Scenario

Add `scripts/smoke-receipt-parsing.sh`:

- requires `API_BASE_URL` and `AUTH_TOKEN`;
- creates or expects at least one category;
- uploads a tiny generated PNG receipt image;
- polls active/get until `ready`;
- approves returned draft expenses;
- checks that approve response status is `approved`.

## Acceptance Criteria

- `rtk go test ./...` passes.
- The receipt parser no longer depends on process memory for queued work.
- A process restart after job creation can be recovered because file bytes are persisted.
- Stale processing jobs can be moved back to queued.
- The smoke script documents the live API happy path.
