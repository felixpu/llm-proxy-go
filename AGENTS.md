# Repository Guidelines

## Project Structure & Module Organization
- `cmd/llm-proxy/`: application entrypoint and startup wiring.
- `internal/`: core backend code by layer:
  - `api/handler`, `api/middleware` for HTTP routes and cross-cutting concerns.
  - `service/` for business logic (routing, proxying, health checks, cache).
  - `repository/` for DB access; `database/migrations/` for schema changes.
  - `config/`, `models/`, `version/`, `pkg/` for support modules.
- `frontend/`: embedded static assets (CSS/JS/Vue pages, vendor libs, `openapi.yaml`).
- `tests/e2e/`: end-to-end suites; shared helpers in `tests/testutil/`.
- `docs/decisions/`: ADRs and architecture records.

## Build, Test, and Development Commands
- `make build`: build local binary `./llm-proxy` with version ldflags.
- `make build-all`: cross-compile for linux/darwin/windows (amd64/arm64).
- `make test` or `make test-unit`: run unit tests (`./internal/...`).
- `make test-integration`: run integration tests (`-tags=integration`).
- `make test-e2e`: run E2E tests (`-tags=e2e -timeout=120s`).
- `make test-coverage`: generate `coverage.out` and `coverage.html`.
- `go fmt ./... && go vet ./...`: required local hygiene before PR.

## Coding Style & Naming Conventions
- Use standard Go formatting (`gofmt`) and keep imports gofmt-organized.
- Package names: short, lowercase, no underscores.
- File names: snake_case by responsibility (for example `routing_analyzer_v2.go`).
- Tests: colocated with source (`*_test.go`) unless E2E in `tests/e2e/`.
- Keep code comments in English; user-facing docs can be Chinese.

## Testing Guidelines
- Unit tests default to `//go:build !integration && !e2e`.
- Integration tests use `//go:build integration`.
- E2E tests use `//go:build e2e` and should validate public API behavior.
- Prefer `stretchr/testify` assertions for readability.
- Add/adjust tests with every behavior change, especially in `internal/service/` and `internal/repository/`.

## Commit & Pull Request Guidelines
- Follow Conventional Commit style seen in history: `feat(scope): ...`, `fix(scope): ...`, `refactor(scope): ...`, `docs: ...`, `test(scope): ...` (emoji prefix is optional).
- Keep commits focused; avoid mixing refactor and feature logic.
- PRs should include:
  - clear summary and motivation,
  - impacted modules (for example `internal/service/proxy.go`),
  - test evidence (`make test`, tags if used),
  - screenshots/GIFs for frontend changes.
- Link related issues/ADRs when changing architecture or routing behavior.
