# Repository Guidelines

## Project Structure & Module Organization

This Go HTTP API has no frontend. Routes: `cmd/api/`, wiring: `factory/`, handlers: `internal/api/handlers/`, services: `internal/services/`, persistence: `internal/repository/`. Contracts: `internal/dto/`; integrations: `pkg/`. Goose migrations live in `migrations/schema/`.

Read `docs/member-journey.md` before changing business behavior. Update `frontend.md` when an implemented API contract changes. Deliver one agreed feature at a time.

## Architecture & Repository Patterns

Inspect existing services and repositories before extending them.

- Handlers decode/validate DTOs, call services, and write responses. Services own business rules and transaction boundaries, using repository interfaces. Preserve constructor injection and factory wiring.
- Repositories own queries. Default to Squirrel (`sq.StatementBuilder.PlaceholderFormat(sq.Dollar)`) and parameterized values. Specialized queries or raw SQL are appropriate when requirements exceed the existing patterns; keep them in repository methods and explain the exception.
- Define typed `XRepositoryFilter` structs with optional pointer fields. Reuse `buildQuery`/`generateQuery` and `applyFilter` across `Get`, `List`, and `Exists`/count operations; see `internal/repository/users.go` and `members.go`.
- Follow existing `QueryOptions`, `ApplyPagination`, and `ListResult[T]` conventions for lists; preserve soft-delete filtering.
- Repository methods that support transactions accept `tx *sqlx.Tx`. Execute through `tx` when non-nil; otherwise use the repository's database connection. Services begin, commit, and roll back transactions. Extend existing `Get` methods rather than adding duplicate transaction-specific getters.
- Generate database models through `sqlc generate` using `sqlc.yml` and migrations. Regenerate after schema changes; edit generated files through their sources.
- For joined data, follow `transactions.go`, `shares.go`, and `fines.go`: aliased select columns, flat scan structs, `mapFlatToPopulated`, `PopulatedX`, and `GetPopulated`/`ListPopulated`. Convert responses through `MapRepositoryToDTOModel`.

## Build, Test, and Development Commands

- `make build/api`: compile the API.
- `make start/api`: build and run locally.
- `make docker/start`: start API and PostgreSQL.
- `go test ./...`: run unit tests and compilation checks.
- `go vet ./...`: run static checks.
- `go test -tags=integration ./cmd/api`: run PostgreSQL tests with `ARA_TEST_DATABASE_URL`; fixtures use isolated schemas.

## Coding Style & Testing

Use the injected `pkg/logger.Logger`: Zap for typed fields or `Sugar.Infow`/`Errorw` for structured key/value logs. Keep secrets out of logs. Use `gofmt`, lowercase packages, `snake_case.go` filenames, Go identifier conventions, and `snake_case` JSON fields. Pass context first and handle errors early. Tests use Go's `testing` package, `*_test.go`, and `TestXxx`; cover authorization and financial behavior.

## Commit, Pull Request & Configuration Guidelines

Use concise `feat:`/`refactor:` subjects. PRs describe behavior, issues, checks, and migration/configuration changes. Keep secrets in ignored `.env`. Use forward migrations for initialized databases. `make db/seed` resets data; reserve it for disposable development databases.
