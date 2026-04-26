# Tech Debt

## Receipt parsing storage durability

Status: open
Added: 2026-04-25

The async receipt parsing worker no longer depends on an in-process queue for correctness: jobs are claimed from `receipt_parse_jobs`, and uploaded files are persisted through `receipt_parse_files.storage_key`.

Remaining limitation: the current storage implementation is local filesystem storage, configured by `RECEIPT_FILE_STORAGE_DIR`. This is acceptable for local development and a single-pod MVP only if the storage directory is durable for that pod.

Impact:

- if multiple backend pods run without shared storage, a worker pod may claim a job whose receipt file exists only on another pod's filesystem;
- if the local storage volume is lost, queued jobs can still exist in the database but their receipt files cannot be recovered;
- operational backups need to include both PostgreSQL data and the configured receipt file storage directory.

Required production fix:

- replace local filesystem storage with S3-compatible/object storage, or mount a durable shared volume across all worker pods;
- add lifecycle/cleanup policy for files after parse cancellation, failure retention, and successful approval retention;
- include receipt file storage in backup and restore runbooks.

Until this is fixed, receipt parsing should be treated as single-instance MVP behavior, not a fully multi-pod durable storage design.
