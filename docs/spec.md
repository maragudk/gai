# GAI (Go Artificial Intelligence)

## Objective

GAI (`maragu.dev/gai`) is a Go library that standardises interactions with foundational models, large language models, and embedding models across multiple providers. It targets Go developers who want a single, provider-agnostic API for chat completions, embeddings, tool calling, and model evaluation -- without giving up streaming, type safety, or control over tool execution.

The library is pronounced "guy".

GAI is not a framework or agent runtime. It is a minimal abstraction layer: thin enough that provider-specific features remain accessible through the underlying SDK clients, but uniform enough that switching providers or running evaluations across them requires no structural changes.

## Tech Stack

- **Language**: Go (minimum 1.25).
- **Provider SDKs**: `github.com/openai/openai-go/v3`, `github.com/anthropics/anthropic-sdk-go`, `google.golang.org/genai`.
- **Schema generation**: `github.com/invopop/jsonschema` for deriving JSON Schema from Go structs.
- **Observability**: OpenTelemetry (`go.opentelemetry.io/otel`) for tracing chat completion and embedding calls.
- **Evaluation**: Levenshtein distance via `github.com/agnivade/levenshtein`; cosine similarity built-in.
- **Testing**: Standard `go test`; assertions via `maragu.dev/is`.
- **Local inference**: Docker Compose with `maragudk/llama-3.2-1b-instruct-q4_k_m` on port 8090 for smoke tests.
- **CI**: GitHub Actions (test, evaluate, lint).

## Project Structure

```
.
├── chat_complete.go          # Core types: ChatCompleter, Message, Part, Tool, Schema
├── embed.go                  # Core types: Embedder, EmbedRequest, EmbedResponse
├── clients/
│   ├── anthropic/            # Anthropic (Claude) client, chat completer
│   ├── google/               # Google (Gemini) client, chat completer, embedder
│   └── openai/               # OpenAI client, chat completer, embedder
├── tools/
│   ├── time.go               # get_time tool
│   ├── exec.go               # exec tool (shell commands)
│   ├── fetch.go              # fetch tool (HTTP + optional HTML-to-Markdown)
│   ├── memory.go             # save_memory, get_memories, search_memories tools
│   └── file.go               # read_file, list_dir, edit_file tools
├── eval/
│   ├── eval.go               # Scorers: lexical similarity, semantic similarity, exact match, contains
│   └── run.go                # Eval runner (Run, E, Log) integrated with go test
├── internal/examples/
│   ├── evals/                # Runnable eval examples
│   ├── tools/                # Runnable tool-use example
│   └── tools_custom/         # Runnable custom-tool example
├── docs/
│   ├── spec.md               # This file
│   ├── decisions.md          # Architectural decision records
│   ├── index.html            # Public docs site
│   └── template.html         # Docs site template
├── Makefile                  # Build, test, lint, evaluate, benchmark targets
├── docker-compose.yaml       # Local llama inference server
└── evals.jsonl               # Evaluation output (generated, not committed)
```

## Commands

```shell
# Build
go build ./...

# Test (with coverage)
make test
# or: go test -coverprofile cover.out -shuffle on ./...

# View coverage
make cover

# Run evaluations (only tests prefixed with TestEval)
make evaluate
# or: go test -run TestEval ./...

# Lint
make lint
# or: golangci-lint run

# Format
make fmt

# Benchmark
make benchmark
# or: go test -bench . ./...

# Start local inference server
make test-up
# or: docker compose up -d

# Stop local inference server
make test-down
# or: docker compose down
```

## Code Style

- Exported identifiers documented in GoDoc style: sentence starting with the identifier name.
- Dependency injection through small private interfaces close to the consumer (e.g., `memorySaver`, `memoryGetter`, `memorySearcher` in `tools/memory.go`).
- Tool implementations always provide a schema via `gai.GenerateToolSchema[T]()` and implement both `Summarize` and `Execute` functions.
- Schema generation uses `jsonschema_description` struct tags for argument documentation.
- Favour package-private helpers over new exports. Top-level Go files define the public API surface.
- Use `gai.Ptr[T]()` for pointer-to-value conversions in option fields.
- Streaming responses use `iter.Seq2[Part, error]` iterators.
- Provider clients expose the underlying SDK client as a public field for escape-hatch access.

## Testing

- Framework: Standard `go test` with `maragu.dev/is` assertion helpers (`is.NotError`, `is.Equal`).
- Test files live alongside implementation files (`*_test.go`).
- Tests run with `-shuffle on` to catch ordering dependencies.
- Evaluations are Go tests prefixed with `TestEval` and are skipped unless `-run TestEval` is passed.
- Eval results are logged to `evals.jsonl` at the project root in JSONL format.
- Integration tests use real provider APIs via environment variables (`OPENAI_API_KEY`, `GOOGLE_API_KEY`, `ANTHROPIC_KEY`).
- Local smoke tests can use the Docker Compose llama server via the OpenAI client with a custom `BaseURL`.

## Git Workflow

- Main branch: `main`.
- CI runs on push to `main` and on pull requests targeting `main`.
- CI jobs: test, evaluate (skipped for Dependabot), lint (skipped for Dependabot).
- Concurrency: one CI run per branch, in-progress runs cancelled for non-main branches.
- Dependabot manages dependency updates.

## Boundaries

- **Do not expand the top-level public API surface** without a clear, demonstrated need. The core types in `chat_complete.go` and `embed.go` are intentionally minimal.
- **Do not call tools automatically**. Tool execution is always the caller's responsibility -- this is a deliberate design choice, not a missing feature.
- **Do not add new provider clients** without matching the existing pattern: client struct with options, `NewChatCompleter`/`NewEmbedder` factory methods, interface satisfaction checks (`var _ gai.ChatCompleter = ...`).
- **Do not modify `docs/index.html`** without running `go test` first to verify examples.
- **Do not commit `evals.jsonl`** -- it is regenerated on each evaluation run.
- **Deprecated aliases** (e.g., `MessagePart`, `MessagePartType`) exist for backwards compatibility. Do not add new ones; remove existing ones when the next major version allows.
