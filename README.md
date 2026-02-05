# family-app-go

Go backend for family-app.

## Run

```bash
HTTP_PORT=8080 go run ./cmd/family-app
```

## Env

- `HTTP_PORT` (default `8080`)
- `ENV` (default `development`)
- `DB_DSN` (optional override)
- `DB_HOST` (default `localhost`)
- `DB_PORT` (default `5432`)
- `DB_USER` (default `postgres`)
- `DB_PASSWORD` (default `postgres`)
- `DB_NAME` (default `family_app`)
- `DB_SSLMODE` (default `disable`)
- `DB_TIMEZONE` (default `UTC`)
- `DB_MAX_OPEN_CONNS` (default `10`)
- `DB_MAX_IDLE_CONNS` (default `5`)
- `DB_CONN_MAX_LIFETIME` (default `30m`)
- `SUPABASE_URL` (required)
- `SUPABASE_PUBLISHABLE_KEY` (required)
- `SUPABASE_AUTH_TIMEOUT` (default `5s`)

## Structure

- `cmd/family-app` — entrypoint
- `internal/app` — application wiring
- `internal/config` — env-based configuration
- `internal/db` — database connections
- `internal/transport/httpserver` — HTTP server (chi) and routes
- `internal/transport/httpserver/handler` — HTTP handlers
- `pkg/` — reusable libraries (public)
- `api/` — API specs (OpenAPI)
- `migrations/` — database migrations
- `scripts/` — dev scripts
