# Paisa — Claude Code Guidelines

## File reading efficiency

- When multiple tasks touch the same file, read it **once** before starting the task group and reference that snapshot — don't re-read between steps.
- Use `offset` + `limit` on `Read` for large files when only a section is relevant. `expense/monthly.ts`, `ledger.go`, and `import/+page.svelte` are each 15-30KB; read the relevant function range, not the whole file.
- Prefer `grep` / `find` via Bash for locating symbols before reading full files.

## Backlog and planning

- Full backlog (performance + features) is at `docs/superpowers/backlog.md`. Check it before starting any performance or feature work.
- Plans go in `docs/superpowers/plans/`, specs in `docs/superpowers/specs/`.
- Worktrees live in `.worktrees/` (already git-ignored).

## Project conventions

- Go backend: `internal/server/` for HTTP handlers, `internal/service/` for business logic, `internal/model/` for DB models.
- Frontend: SvelteKit in `src/routes/(app)/`, shared D3 renderers in `src/lib/`.
- Tests: Go tests co-located (`*_test.go`), run with `go test ./internal/...`.
- TypeScript check: `npx tsc --noEmit`, format: `npx prettier --write src/`.
- Build: `make build` (requires Go + Node).
