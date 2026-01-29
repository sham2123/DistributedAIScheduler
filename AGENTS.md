# Repository Guidelines

## Project Structure & Module Organization
- `cmd/` holds the Go entry points (`scheduler`, `worker`, `synctl`), each in its own folder.
- `api/proto/v1/` stores the shared Protobuf definitions; regenerate Go code after editing `.proto` (`protoc --go_out=./ --go-grpc_out=./ ...`).
- `internal/` contains reusable runtime logic (cluster, job, node models); prefer exposing interfaces and keeping concrete state private.
- `scripts/` hosts setup helpers, while the repo root keeps go.mod/sum plus high-level docs (README, AGENTS).

## Build, Test, and Development Commands
- `go mod tidy` refreshes dependencies after changing imports or proto definitions.
- `go test ./...` runs the full unit suite; reference the command in every PR.
- `go run cmd/scheduler/main.go`, `go run cmd/worker/main.go`, and `go run cmd/synctl/main.go` start each component for manual integration checks.
- Re-run `protoc --go_out` / `protoc --go-grpc_out` after editing `.proto` files and keep the generated `.pb.go` files under `api/proto/v1`.

## Coding Style & Naming Conventions
- Follow standard Go formatting: tabs for indentation, `gofmt` (or `go fmt ./...`) for files you touch, and run `go vet ./...` before the final PR.
- Keep exported identifiers in `CamelCase`, internal helpers in `camelCase`, and prefer descriptive names for gRPC requests/responses (`RegisterWorkerRequest`, etc.).
- Organize code so each binary has its own package import path (`cmd/<name>/main.go`) and domain logic lives in `internal/<subsystem>`.

## Testing Guidelines
- Tests should follow Go’s `*_test.go` naming and live next to the code they cover; use table-driven tests where practical.
- Run `go test ./...` locally; for targeted runs, leverage `go test ./internal/scheduler -run TestName`.
- Document new integrations in README or AGENTS so reviewers know how to exercise the behaviour manually.

## Commit & Pull Request Guidelines
- Keep commit messages concise and present tense, e.g., `add worker.proto`, mirroring the existing log style.
- Each PR description should explain what changed, list commands used (tests/builds), and link related issues or RFC notes; include screenshots or logs when demonstrating a UI/CLI change.
- Mention in the PR whether proto/generated files were updated and describe any manual steps needed to reproduce the change.

## Security & Configuration Tips
- Install `protoc`, `protoc-gen-go`, and `protoc-gen-go-grpc` into `$GOPATH/bin` before building.
- Keep secrets out of the repo; review `scripts/` for helpers touching configs, and document env vars for gRPC endpoints in `README.md`.
