---
name: family-app-go-github-pr
description: Use when working in the family-app-go repository and the user asks to publish changes to GitHub, push a branch, create or update a pull request, prepare a PR body, or finish backend work for review.
metadata:
  short-description: Publish family-app-go changes and open PRs
---

# family-app-go GitHub PR

Use this skill only for `/Users/ashpak/Pet/family-app-go`.

## Goal

Publish the current backend change to GitHub through a feature branch and create a focused PR into `main`.

Do not push directly to `main` unless the user explicitly asks. A push to `main` triggers production deploy through `.github/workflows/deploy.yml`.

## Preflight

1. Confirm repository context:
   - `git rev-parse --show-toplevel`
   - `git branch --show-current`
   - `git remote -v`
   - `git status --short`
2. Identify unrelated dirty files before staging. Treat existing uncommitted work as user-owned unless it is clearly part of the current task.
3. If the branch is `main`, create a feature branch before committing. Prefer `codex/<short-task-slug>` unless the user supplied a branch name.
4. Fetch before publishing:
   - `git fetch origin`
5. Verify the branch is based on current `origin/main` when practical. If it is not, explain the divergence and avoid rebasing unless the user asks.

## Validation

Run validation before committing or opening the PR.

- For Go changes, run `gofmt -w` on touched `.go` files.
- Run `go test ./...`.
- Run e2e tests only when the user asks or a valid `E2E_DB_DSN` is available:
  - `go test -tags e2e ./e2e/...`
- If migrations, environment variables, OpenAPI, Docker, or deployment behavior changed, call that out in the PR body.

If validation fails, stop before push/PR unless the user explicitly wants a draft PR with failing checks documented.

## Commit

1. Stage only the files that belong to the requested change:
   - `git add <file> ...`
2. Inspect staged changes:
   - `git diff --cached --stat`
   - `git diff --cached`
3. Commit with a short imperative subject. Use a conventional prefix when it fits, for example:
   - `feat: add receipt correction memory`
   - `fix: handle empty receipt parse result`
   - `docs: describe receipt parser setup`

Do not commit `.env`, local secrets, database dumps, temporary logs, or unrelated generated files.

## Push And PR

1. Push the feature branch:
   - `git push -u origin <branch>`
2. Create the PR with GitHub CLI when available:
   - `gh pr create --base main --head <branch> --title "<title>" --body "<body>"`
3. If `gh` is unavailable or unauthenticated, push the branch and give the user the exact GitHub compare URL.

Use this PR body shape:

```markdown
## Summary
- <what changed>
- <why it matters>

## Tests
- `go test ./...`
- <other checks, or "Not run: <reason>">

## Notes
- <env, migration, API, deploy, or compatibility notes; omit if none>
```

For PR titles, keep them concise and scoped to one change.

## Final Response

Report:

- branch name
- commit hash
- PR URL or compare URL
- validation commands and their results
- any unrelated files intentionally left untouched

If files were staged, committed, pushed, or a PR was created through Codex App git actions, emit the corresponding git directive in the final answer.
