# GAI Project Guidelines

## Build & Test Commands
- Build: `go build`
- Lint: `make lint` (uses golangci-lint)
- Test all: `make test` (shuffle on, coverage profile)
- Test single: `go test -run TestName ./...`
- Run evals: `make evaluate` or `go test -run TestEval ./...`
- Coverage report: `make cover`
- Benchmarks: `make benchmark`
- Docker test env: `make test-up` / `make test-down`

## Code Style Guidelines
- **Imports**: Group by standard lib, third-party, internal; alphabetical within groups
- **Formatting**: gofmt standard, tabs for indentation
- **Types**: Use generics with constraints, type aliases for domain concepts
- **Naming**: PascalCase (exported), camelCase (unexported), descriptive names
- **Interfaces**: Small, focused, ending with '-er', verified with `var _ Interface = (*Implementation)(nil)`
- **Error handling**: Explicit checks, return errors to callers, use `is.NotError` in tests
- **Patterns**: Functional patterns (iterators, yield functions), context for cancellation
- **Comments**: Godoc-style comments for public APIs
- **Testing**: Table-driven tests, helper functions with t.Helper(), use maragu.dev/is for assertions, prefer integration tests over mocks, use subtests with `t.Run`
