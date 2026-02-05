# family-app-go

Go backend for family-app.

## Run

```bash
HTTP_PORT=8080 go run ./cmd/family-app
```

## Tests

Unit tests:

```bash
make test
```

E2E tests (require Postgres DSN):

```bash
E2E_DB_DSN="postgres://user:pass@localhost:5432/family_app?sslmode=disable" make e2e
```

## Migrations

On startup, the service applies SQL migrations from `migrations/` in filename order and records them in `schema_migrations`.

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
- `internal/domain` — domain/business logic
- `internal/repository` — data access layer
- `internal/transport/httpserver` — HTTP server (chi) and routes
- `internal/transport/httpserver/handler` — HTTP handlers
- `pkg/` — reusable libraries (public)
- `api/` — API specs (OpenAPI)
- `migrations/` — database migrations
- `scripts/` — dev scripts


## Supabase Auth (RU)

Коротко: фронт получает токен от Supabase, бэк валидирует его через `SUPABASE_URL` + `SUPABASE_PUBLISHABLE_KEY`.

**Env**
- `SUPABASE_URL` — Project URL из Supabase.
- `SUPABASE_PUBLISHABLE_KEY` — `anon/public` key из Supabase.
- `SUPABASE_AUTH_TIMEOUT` — таймаут запроса к Supabase Auth (опционально).

**Supabase Dashboard**
1. `Project Settings -> API`: возьми `Project URL` и `anon/public` key.
2. `Authentication -> URL Configuration`: задай `Site URL` (prod) и `Additional Redirect URLs` (dev/prod коллбеки), например `http://localhost:5173/auth/callback` и `https://app.example.com/auth/callback`.
3. `Authentication -> Providers -> Google`: включи провайдера и вставь `Client ID` и `Client Secret` из Google Cloud Console.

**Google Cloud Console**
1. Создай проект и экран согласия OAuth (OAuth consent screen).
2. Создай `OAuth Client ID` типа **Web application**.
3. `Authorized JavaScript origins`: добавь фронтовые origins, например `http://localhost:5173` и `https://app.example.com`.
4. `Authorized redirect URIs`: добавь **только** callback Supabase вида `https://<project-ref>.supabase.co/auth/v1/callback`.
5. Скопируй `Client ID`/`Client Secret` в Supabase Dashboard.

**Redirect URL / Origin URL**
- Redirect URL — это путь на фронте, куда Supabase возвращает пользователя после логина. Он должен быть в `Additional Redirect URLs`.
- Origin URL — домен фронта; добавь его в `Authentication -> URL Configuration -> Site URL` (prod).
- Origin URL нужно добавить в `Authorized JavaScript origins` в Google Cloud Console.
- Origin URL нужно добавить в CORS-ориджины бэка (см. `internal/transport/httpserver/routes.go`).
