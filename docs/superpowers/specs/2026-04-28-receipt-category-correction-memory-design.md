# Receipt Category Correction Memory Design

## Goal

Add family-specific category correction memory for receipt item parsing.

When a user approves a receipt parse after manually changing an item category, the backend records that correction for the family. Future receipt parsing requests pass compact, family-specific hints to the OpenAI parser so similar items are more likely to be classified into the corrected category.

This is a soft hint system, not a fine-tuning flow and not a hard rule engine.

## Current Context

The backend already supports asynchronous receipt parsing:

- `POST /api/receipt-parses` creates a queued parse job and stores one uploaded receipt image.
- A DB-backed worker processes queued jobs and calls the configured receipt parser.
- Parsed line items are persisted in `receipt_parse_items`.
- Draft expenses are aggregated by item category.
- Users can review item-level amount/category state with `PATCH /api/receipt-parses/{id}/items`.
- `POST /api/receipt-parses/{id}/approve` transactionally creates final expenses.
- Receipt files are deleted after successful approve/cancel.

The important existing code paths are:

- `internal/domain/receipts/model.go`
- `internal/domain/receipts/repository.go`
- `internal/domain/receipts/service.go`
- `internal/repository/postgres/receipts/postgres.go`
- `internal/repository/http/receipts/openai.go`
- `migrations/0022_create_receipt_parse_tables.sql`
- `migrations/0023_add_receipt_parse_worker_state.sql`

## Recommended Architecture

Implement the memory system in three persisted layers:

1. Raw correction events
2. Canonical family hints
3. Hint examples

Approve writes only raw correction events inside the existing receipt approve transaction. It does not create derived hints, does not call an LLM, and does not depend on any external API. This keeps expense creation transactional and predictable.

Derived hints are created by a separate DB-backed materializer worker. The worker reads unprocessed correction events, asks a cheaper text model to match an existing family hint or propose a new canonical name, and writes the canonical hint/example tables. The default model is `gpt-5.4-nano`, configured separately from the image receipt parser model. If the model is unavailable, returns invalid output, or produces a low-confidence result, the worker falls back to deterministic canonicalization.

## Data Model

### Raw Correction Events

Table: `receipt_parse_category_correction_events`

Purpose: immutable-ish log of confirmed user category corrections.

Columns:

- `id uuid primary key`
- `family_id uuid not null references families(id) on delete cascade`
- `user_id uuid not null`
- `receipt_parse_job_id uuid not null references receipt_parse_jobs(id) on delete cascade`
- `receipt_parse_item_id uuid not null references receipt_parse_items(id) on delete cascade`
- `source_item_text text not null`
- `normalized_item_text text not null`
- `llm_category_id uuid null`
- `final_category_id uuid not null references categories(id) on delete cascade`
- `processed_at timestamptz null`
- `materialize_attempt_count int not null default 0`
- `last_materialize_attempt_at timestamptz null`
- `next_materialize_attempt_at timestamptz null`
- `locked_at timestamptz null`
- `locked_by text null`
- `materialize_error_code text null`
- `materialize_error_message text null`
- `created_at timestamptz not null default now()`

Indexes:

- `(family_id, created_at desc)`
- `(processed_at) where processed_at is null`
- `(processed_at, locked_at, next_materialize_attempt_at, created_at) where processed_at is null`
- unique `(receipt_parse_item_id)` to avoid duplicate correction events for the same approved item

Event creation rule:

- Skip deleted items.
- Require `final_category_id`.
- Create an event when `llm_category_id` is null and `final_category_id` is set.
- Create an event when `llm_category_id` differs from `final_category_id`.
- Do not create an event when the model category and final category are the same.

`EditedByUser` is not the source of truth for this decision because item edits can change amount without changing category.

### Canonical Family Hints

Table: `receipt_parse_family_hints`

Purpose: compact prompt-ready hints for a family.

Columns:

- `id uuid primary key`
- `family_id uuid not null references families(id) on delete cascade`
- `canonical_name text not null`
- `final_category_id uuid not null references categories(id) on delete cascade`
- `times_confirmed int not null default 1`
- `last_confirmed_at timestamptz not null`
- `created_at timestamptz not null default now()`
- `updated_at timestamptz not null default now()`

Indexes:

- unique `(family_id, canonical_name, final_category_id)`
- `(family_id, times_confirmed desc, last_confirmed_at desc)`
- `(family_id, final_category_id)`

MVP canonicalization:

- Use the item's normalized name if present.
- Otherwise use the raw item name.
- Trim whitespace.
- Store the value as `canonical_name`.
- Upsert by `(family_id, canonical_name, final_category_id)`.
- On conflict, increment `times_confirmed`, update `last_confirmed_at`, and update `updated_at`.

### Hint Examples

Table: `receipt_parse_family_hint_examples`

Purpose: evidence for a canonical hint.

Columns:

- `id uuid primary key`
- `hint_id uuid not null references receipt_parse_family_hints(id) on delete cascade`
- `correction_event_id uuid not null references receipt_parse_category_correction_events(id) on delete cascade`
- `source_item_text text not null`
- `normalized_item_text text not null`
- `created_at timestamptz not null default now()`

Indexes:

- unique `(hint_id, correction_event_id)`
- `(hint_id, created_at desc)`

## Domain Model Changes

Add receipt domain structs:

- `CategoryCorrectionEvent`
- `FamilyHint`
- `FamilyHintExample`
- `CorrectionHint`

Extend `ParseReceiptInput` with:

```go
Corrections []CorrectionHint
```

`CorrectionHint` contains:

```go
type CorrectionHint struct {
    CanonicalName  string
    CategoryID     string
    CategoryName   string
    TimesConfirmed int
}
```

## Repository Changes

Extend `receipts.Repository` with methods that support the approved flow and prompt injection:

- `CreateCategoryCorrectionEvent(ctx, event *CategoryCorrectionEvent) error`
- `AcquireUnprocessedCategoryCorrectionEvent(ctx, workerID string, now time.Time) (*CategoryCorrectionEvent, error)`
- `RequeueStaleCategoryCorrections(ctx, staleBefore time.Time) (int64, error)`
- `MarkCategoryCorrectionEventProcessed(ctx, eventID string, processedAt time.Time) error`
- `ReleaseCategoryCorrectionEventWithError(ctx, eventID, code, message string, nextAttemptAt *time.Time) error`
- `UpsertFamilyHint(ctx, input UpsertFamilyHintInput) (*FamilyHint, error)`
- `CreateFamilyHintExample(ctx, example *FamilyHintExample) error`
- `ListFamilyHints(ctx, familyID string, categoryIDs []string, limit int) ([]FamilyHint, error)`

The exact input type names can follow existing Go style during implementation, but the behavior must remain the same.

## Approve Flow

`ApproveParse` currently loads job, draft expenses, and items before opening the transaction. It then creates final expenses, updates drafts, and marks the job approved inside one transaction.

The raw correction event write should happen in that same transaction after final expense creation succeeds and before the job is marked approved.

For each item:

1. Skip deleted items.
2. Skip items without `FinalCategoryID`.
3. Skip items where `LLMCategoryID` equals `FinalCategoryID`.
4. Create a `CategoryCorrectionEvent`.

If there are no category corrections, approve behavior is unchanged.

If correction persistence fails, approve should fail and roll back with the rest of the transaction because this is local database work inside the same business operation. The flow must not call an LLM.

## Hint Materialization Flow

The receipt service owns a second background loop next to the existing receipt parser worker:

1. Recover stale locked correction events on startup.
2. Acquire one unprocessed correction event using row locking and `SKIP LOCKED`.
3. Load allowed category metadata for the event family.
4. Load existing hints for the same family and final category.
5. Ask the configured `HintNormalizer` for either a match to an existing hint or a new canonical name.
6. Validate the result.
7. In one DB transaction, upsert/increment the selected canonical hint, create a hint example, and mark the event processed.

Retries:

- Transient normalizer or DB errors release the event with error metadata and a future `next_materialize_attempt_at`.
- After the configured max attempts, the worker uses deterministic fallback and marks the event processed if the local DB writes succeed.

Fallback:

- Use the event's `normalized_item_text` if present.
- Otherwise use `source_item_text`.
- Upsert by `(family_id, canonical_name, final_category_id)`.

Low-confidence LLM output also falls back deterministically.

## Prompt Injection Flow

In `processJob`:

1. Resolve allowed categories for the job.
2. Build the allowed category ID list.
3. Load top family hints with `ListFamilyHints`.
4. Filter at the repository/query level to only hints whose `final_category_id` is in the allowed category set.
5. Pass the hints into `ParseReceiptInput.Corrections`.

Ranking:

- `times_confirmed desc`
- `last_confirmed_at desc`

Limit:

- 20 hints.

## OpenAI Parser Prompt

The OpenAI request schema should remain unchanged.

When correction hints are present, append a user prompt block with this meaning:

```text
Family-specific category hints:
- "Коктейль Exponenta" -> "Спорт"
- "Протеиновый батончик Bombbar" -> "Спорт"

Use these as soft hints only.
Do not treat them as strict rules.
If the current receipt item is clearly different, ignore the hint.
```

The prompt must include category names for model readability while the JSON schema continues to restrict output to allowed category IDs.

## LLM Hint Normalizer

Add a domain interface:

```go
type HintNormalizer interface {
    NormalizeCategoryCorrection(ctx context.Context, input NormalizeCategoryCorrectionInput) (*NormalizeCategoryCorrectionResult, error)
}
```

The OpenAI implementation uses `gpt-5.4-nano` by default, calls the Responses API with structured JSON output, and returns:

- `action`: `match_existing` or `create_new`
- `hint_id`: required only for `match_existing`
- `canonical_name`: required for `create_new`
- `confidence`: number from 0 to 1

Validation rules:

- A matched `hint_id` must be one of the existing hints sent in the request.
- A matched hint must have the same final category.
- A new canonical name must be non-empty after trimming.
- Confidence below the configured threshold falls back to deterministic canonicalization.

This normalizer is asynchronous from the user's perspective. It must not be called synchronously from approve.

## Testing Strategy

Focused domain service tests:

- Changed category on approve creates a raw correction event.
- Uncategorized LLM item manually categorized creates a raw correction event.
- Unchanged category does not create a correction event.
- Approve does not directly create derived hints.
- Materializer creates a new canonical hint from LLM output.
- Materializer matches an existing hint and increments `times_confirmed`.
- Low-confidence materializer output uses deterministic fallback.
- Normalizer error retries without marking the event processed.
- Repeated fallback correction increments `times_confirmed`.
- Parser input receives top hints during job processing.
- Hints are filtered by allowed categories.

OpenAI parser tests:

- Prompt includes the family-specific hints block when corrections are provided.
- Prompt omits the block when no corrections are provided.
- Strict output schema remains category-ID constrained.

Postgres repository behavior should be covered through focused repository tests if an existing local DB test pattern is available. If no such pattern exists for receipt repository tests, service tests with the fake repository should cover behavior and full `go test ./...` should verify compilation.

## Non-Goals

- Do not fine-tune a model.
- Do not make hints strict rules.
- Do not block approve on an external LLM call.
- Do not add a new public API for managing hints in this iteration.
- Do not change the receipt parser JSON output schema.
