# GAI Coding Agents Playbook

## What This Project Does
The repository implements `maragu.dev/gai`, a Go library that standardises interactions with foundational and large language models. Core packages cover chat completions (`chat_complete.go`), embedding helpers (`embed.go`), reusable tools (`tools/`), and an evaluation harness (`eval/`). Example programs live under `internal/examples` to illustrate end-to-end usage with different backends.

## Repository Landmarks
- Top-level Go files expose the public API; keep additional exports minimal.
- `tools/` packages convenience tools (time, exec, fetch, memory, file) with matching tests and JSON schemas.
- `eval/` provides the evaluation runner that writes JSONL reports to `evals.jsonl`; `eval/internal` holds scorer utilities.
- `internal/examples/` contains runnable samples (`evals`, `tools`, `tools_custom`) demonstrating library integration.
- `docs/` stores the static site (`index.html`, `template.html`); update it when the public API changes.
- `docker-compose.yaml` starts a local `llama32-1b` inference server on port 8090 for smoke testing chat flows.

## Testing & Quality Gates
- Default test command: `go test -shuffle on ./...` or `make test` (also updates `cover.out` for `go tool cover`).
- Use the `maragu.dev/is` assertion helpers (`is.NotError`, `is.Equal`, etc.) and favour subtests with descriptive names.
- Evaluations run via `go test -shuffle on -run TestEval ./...` or `make evaluate`; logs accumulate in `evals.jsonl`.
- Benchmarks live alongside tests and run with `make benchmark`.
- Lint with `golangci-lint run` or `make lint`; address warnings immediately to avoid CI regressions.

## Coding Conventions
- Stick to dependency injection through small private interfaces close to the consumer (see chat completer tools).
- Add tests for new behaviours; prefer integration-style tests when real dependencies are available.
- For tools, always provide schemas with `gai.GenerateToolSchema` and implement both `Summarize` and `Execute`.
- Document exported identifiers in GoDoc style: start with the identifier name and write a full sentence.
- Avoid introducing new global exports without a clear need; favour package-private helpers inside existing packages.

## Handy References
- Public docs publish from `docs/index.html`; run `go test` before updating to ensure examples remain accurate.
- Utility scripts and sample agents live in `internal/examples/tools*`; reuse them as scaffolding for new samples.
- When adding new make targets or scripts, update both `Makefile` and this guide so future agents stay aligned.
