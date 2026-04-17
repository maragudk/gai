# Diary: robust chat completer

Adding a new subpackage `maragu.dev/gai/robust` that wraps a prioritized list of `gai.ChatCompleter` implementations with jittered exponential-backoff retries and cascading fallbacks. Goal: adopters get a single dependable `ChatCompleter` that handles transient provider failures without having to glue retry + fallback code together themselves. Work happened entirely on branch `robust-chat-completer` off `main`.

## Step 1: Brainstorm and design

### Prompt Context

**Verbatim prompt:** "I want to create a helper implementation of gai.ChatCompleter that can handle both retries and fallbacks, given a prioritized list of gai.ChatCompleter implementations. Retries should be the usual with exponential backoff with jitter. Let's brainstorm."

**Interpretation:** The user wants a brainstorm session before any code — figure out the shape of the API, the streaming semantics, the classifier design, the package location, then write it up.

**Inferred intent:** De-risk the design by exploring tradeoffs upfront. The user is senior enough that they want alignment on subtleties (streaming retry semantics, SDK coupling, observability) before implementation locks anything in.

### What I did

Ran the `fabrik:brainstorm` skill through eight focused Q/A rounds, then consolidated into a design doc at `/docs/design/robust-chat-completer.md`. Questions covered: one combined type vs composable Retrier+Fallbacker; streaming error handling (pre-stream vs mid-stream); error classification (dumb, built-in, pluggable, pluggable-with-defaults); API shape and config granularity; package location and naming; `Retry-After` handling; observability; backoff formula + defaults.

### Why

Streaming is the part of the contract most likely to be gotten wrong. A naive retry wrapper that re-runs the underlying call on any error would duplicate partial output to the caller. Getting the commit-on-first-part decision nailed down in brainstorm meant the implementation loop had one fewer big question to answer under time pressure.

### What worked

The Q/A format produced crisp decisions: one wrapper type (so callers with a single completer still get retries), commit on first part, pluggable classifier with a sensible default, global config across completers, subpackage `robust` (initially top-level was floated but subpackage read better at call sites), no `Retry-After` handling for now, otel spans + slog.Debug only, full jitter with sensible defaults, `MaxAttempts` naming.

### What didn't work

Nothing in this step; the brainstorm flowed cleanly.

### What I learned

`ChatCompleteResponse` uses `iter.Seq2[Part, error]` with a mutable `Meta` pointer that the underlying implementation updates as the stream progresses. That made the "peek first part and return a wrapped response" pattern the natural implementation, because we can forward the same `Meta` pointer to the caller and the underlying implementation keeps mutating it.

### What was tricky

The "one type vs composable" question. Composable (`Fallback(Retry(A), Retry(B))`) is more flexible but the user explicitly wanted one type and pointed out that passing a single completer still gives retry-only behavior, which reconciles the simplicity.

### What warrants review

The design doc was updated twice during implementation (after the SDK-coupling decision and again after code review); the final version at `/docs/design/robust-chat-completer.md` is the source of truth, not the original brainstorm notes.

### Future work

Issue #210 for a gai-native error type (so the default classifier can use interface matching instead of regex). The brainstorm surfaced this naturally when we hit the question "can the default classifier import provider SDKs?" — and the answer was no.

## Step 2: Initial implementation (TDD)

### Prompt Context

**Verbatim prompt:** "Start implementation."

**Interpretation:** The design was agreed; begin writing code. Red/green TDD was called out during brainstorm because the streaming commit-point logic has corner cases.

**Inferred intent:** Build the package incrementally, test-first, one scenario at a time.

### What I did

Created `/robust/` with three files:
- `/robust/chat_completer.go` — types, constructor, `ChatComplete`, `commitOnFirstPart`, `sleep`.
- `/robust/classify.go` — `DefaultErrorClassifier`, `findStatusCode`, `classifyStatus`.
- `/robust/chat_completer_test.go` — external `package robust_test`, `fakeChatCompleter` test double, subtests.

Followed TDD strictly: wrote one failing subtest per scenario (succeeds on first try → retries pre-stream error → retries on iterator-error-before-first-part → passes through mid-stream error after commit → classifier-driven fallback → retry exhaustion cascade → context cancellation → exhaustion → defaults → empty panic → Meta forwarding), watched each fail, then added minimal code to pass. Implementation used `iter.Pull2` to peek the first streamed part synchronously; observability via `otel.Tracer("maragu.dev/gai/robust")` with root + per-attempt spans.

### Why

The streaming commit-point is the most subtle piece — `iter.Pull2`, Meta pointer forwarding, stop() lifecycle, context cancellation during backoff, attempt counter across cascading completers. TDD kept each of these honest.

### What worked

The test-double approach with a queue of `fakeResponse` values (each either `preStreamErr`, or `parts + optional iterErr`) made it trivial to express every scenario. Running `go test -shuffle on ./robust/` after each change gave fast, deterministic feedback.

### What didn't work

First lint failure:

```
robust/classify.go:32:22: stdversion: errors.AsType requires go1.26 or later (file is go1.25) (govet)
	if sc, ok := errors.AsType[statusCoder](err); ok {
```

The module is on Go 1.25.0 per `/go.mod`, but I'd defaulted to the newer `errors.AsType` generic. Switched to the pre-1.26 pattern:

```go
var sc statusCoder
if errors.As(err, &sc) {
    return classifyStatus(sc.StatusCode())
}
```

Tests and lint went clean after the swap.

### What I learned

`errors.AsType[T](err) (T, bool)` landed in Go 1.26. When targeting 1.25, `errors.As(err, &target)` with a pointer argument is the supported pattern.

The gai clients style: tracer namespace matches the package path (`otel.Tracer("maragu.dev/gai/clients/openai")` etc.), span kinds mostly `SpanKindClient` for outbound API calls.

### What was tricky

The commit-on-first-part flow. `iter.Pull2` spawns a goroutine to drive the push iterator; `stop()` releases it. On the failure paths (empty stream, error before first part), we call `stop()` explicitly before returning the error. On the success path, the wrapper's `defer stop()` runs when the caller finishes consuming. This meant `ChatComplete` could no longer return instantly on provider success — it had to block until the first streamed token arrived. That's morally equivalent to waiting for response headers, which is the right moment to commit, but it's a behavior change worth calling out to future readers.

### What warrants review

- `/robust/chat_completer.go` `commitOnFirstPart` — the happy path buffers the first part and then delegates; the error paths tear down `iter.Pull2` before returning.
- The attempt/retry loop in `ChatComplete` — action switch, backoff only between retries (not after the final attempt), fallback breaking the inner loop.

### Future work

Observability still felt thin (root span + per-attempt child); more attributes would help. Deferred for reviewer input.

## Step 3: First-round PR review (inline comments)

### Prompt Context

**Verbatim prompt:** "Make a PR" then later `/fabrik:address-code-review` after the user pushed a `review.jsonl` file with 11 inline comments.

**Interpretation:** Open PR #209, then work through the reviewer's inline comments one at a time via the `address-code-review` skill.

**Inferred intent:** Lock in the implementation with light-touch polish based on the reviewer's direct feedback before rebuilding anything larger.

### What I did

Opened PR #209 via `gh pr create`. Then ran `/fabrik:address-code-review`, read `/review.jsonl`, and worked through the 11 comments one per message. Each accepted comment turned into a small edit plus a one-line deletion from `/review.jsonl`, committed together. Commits in order: mention jitter in package doc → add `_ fmt.Stringer = Action(0)` interface check → simplify defaults docs ("Defaults to 3", etc.) → rename `Classifier` option field to `ErrorClassifier` → drop `trace.SpanKindClient` on the root span (internal is the default) → replace the fake completer's `panic` with `t.Fatalf` via a `newFakeChatCompleter(t, name, responses)` constructor → use `net/http` status constants in `classifyStatus`. `/review.jsonl` was deleted when empty.

### Why

Each comment improved clarity or consistency. The fake-completer panic→t.Fatalf change in particular is the kind of thing that keeps test output readable when something genuinely goes wrong. Renaming `Classifier` → `ErrorClassifier` matched the surrounding naming (`ErrorClassifierFunc`) and made the field self-documenting without a type lookup.

### What worked

The review.jsonl format with `base`/`compare` SHAs, `file`, `startLine`/`endLine`, and `comment` gave enough context to apply each change surgically. The one-comment-per-commit discipline made the PR history easy to read after-the-fact.

### What didn't work

Converting the 17 fake-completer literal sites to the new helper was mechanical but tedious: 17 Edit operations, each site uniquely worded. A sed-style bulk transform would have been faster but less precise.

### What I learned

`trace.SpanKindInternal` is the default when `trace.WithSpanKind` is omitted. The explicit `SpanKindClient` on the root robust span was actually misleading — it's the underlying provider that's the HTTP client; the robust wrapper is orchestration.

### What was tricky

None of the individual changes were tricky; the challenge was only discipline — present one at a time, wait for approval, apply + commit, move on.

### What warrants review

The rename from `Classifier` to `ErrorClassifier` also needed a sweep through `/docs/design/robust-chat-completer.md` and the internal tests; the commit "Rename Classifier field to ErrorClassifier for explicitness" covers all of those.

### Future work

None from this round — pure cleanup.

## Step 4: Second-round review (competing agents)

### Prompt Context

**Verbatim prompt:** `/fabrik:code-review` — dispatches two subagents as competing reviewers. Then `/fabrik:address-code-review` to walk through their findings.

**Interpretation:** The user wanted a deeper audit than inline comments — architectural review, bug-hunting, test-gap analysis. Competing reviewers produce redundant findings that confirm real issues, plus unique ones.

**Inferred intent:** Ship a PR that's been stress-tested before merge, not just polished.

### What I did

Two `general-purpose` subagents ran in parallel against the branch. They returned 15 distinct findings between them — 9 overlaps (higher-confidence real bugs) and 6 unique. I consolidated into a severity-ordered list, triaged one at a time with the user. Of 15 findings, 13 were applied in a single batch commit, 2 skipped with reasoning.

Significant changes in the batch:
- `MaxAttempts < 0`, negative `BaseDelay`/`MaxDelay`, `MaxDelay == math.MaxInt64`, and `BaseDelay > MaxDelay` now all panic at construction.
- Added `ActionNone` as the zero value of `Action`; `tryOnce` returns it on success. The switch gained a `default: panic` that catches both unknown custom-classifier returns AND any bug where `ActionNone` leaks into the retry/fallback switch.
- Backoff formula was off-by-one: first retry slept `rand[0, 2*BaseDelay]` due to shifting by 1-indexed `retryNumber`. Fixed by shifting by `retryNumber-1`, so first retry caps at `BaseDelay`.
- `iter.Pull2` goroutine leak when the caller never iterates — documented the drain contract on `ChatComplete` and filed issue #211 for the real fix.
- Deleted the `statusCoder` interface match in `defaultErrorClassifier`: it was dead code against current SDK error types (all three expose status as a struct field). The regex path is now the only non-context rule, with a tighter pattern `(?:^|[^\w./:])([45]\d{2})(?:[^\w./:]|$)` that rejects adjacency to `:`, `.`, `/`, or digits (ports, IPs, path segments, longer numbers).
- `DefaultErrorClassifier` unexported to `defaultErrorClassifier`. Classifier tests moved from external `chat_completer_test.go` into a new internal `classify_test.go`.
- Empty-iterator case (`!ok` on first pull) now returns an anonymous error so the classifier can retry, instead of committing a zero-part success.
- OTel span lifetime extended: on the success path, both attempt and root spans stay open until the wrapped iterator terminates. `ai.robust.action = "success"` on the successful attempt for trace-filter parity.
- Extracted `nextDelay(retryNumber)` helper so jitter bounds can be table-tested in the internal test file.
- New external tests: `MaxAttempts=1` disables retry, empty-stream retried, unknown-Action panics, negative `MaxAttempts` panics, `BaseDelay > MaxDelay` panics.
- Design doc `/docs/design/robust-chat-completer.md` rewritten to reflect the new state (no `statusCoder`, `ActionNone` in the enum, corrected backoff formula, extended span lifetime, internal+external test split, references to issues #210 and #211).

Skipped findings: (1) extra `slog.Debug` on `ActionFail` — otel already captures it, keeping slog sparse as designed; (2) constructor returning error vs. panicking — consistent with the other panics and with `gai.DataPart`, no reason to break the pattern.

Also posted a PR comment summarizing what was decided, at the user's request.

### Why

The first-round review was line-level polish; this round was architecture + implementation bugs. Issues like the backoff off-by-one and the `tryOnce`-returns-Retry-on-success footgun aren't catchable by inline commenting on individual lines — they need someone thinking about the whole flow.

### What worked

Competing reviewers. Nine overlapping findings (out of 15 total) gave strong signal on which issues were genuinely real rather than a single reviewer's opinion. The user triaged each finding with a one-word answer in most cases, which kept the loop tight.

### What didn't work

One lint failure mid-batch when I first added the `ActionNone` constant:

Not actually — tests + lint went green on the first shot after the batch edits. The earlier fmt nit caught during `make fmt` was just goimports reordering a field I'd touched, nothing substantive.

### What I learned

`errors.As(err, &target)` with a target of interface type (like the old `statusCoder`) works structurally: it matches any error in the tree that satisfies the interface. Useful for capability-style probing without importing concrete types — but only if the target type is satisfiable by the errors being probed, which wasn't the case for the three shipping SDK types.

The regex tightening pattern `(?:^|[^\w./:])([45]\d{2})(?:[^\w./:]|$)` handles ports (`:443`), IP octets (`10.0.0.500`), path segments (`/503/`), and extended digits (`5003`, `44321`) without any prefix-word assumption. Works because Go's RE2 doesn't support lookaround — we capture the surrounding character as context and extract just the match group.

### What was tricky

The span-lifetime extension. Originally both root span and attempt span had `defer span.End()` at the top of `ChatComplete` / `tryOnce`. To keep them open until the stream drained on the success path, I refactored to explicit `End()` calls in the various failure paths and deferred the success-path End inside the wrapped iterator's defer. The attempt span has to be passed into `commitOnFirstPart` so it can End on its own failure paths AND hand off to the wrapper defer on success — ended up passing both spans into `commitOnFirstPart`. Not elegant but clear.

The `ActionNone` zero value also required updating the `String()` method to return "none" and updating the iota order so `ActionRetry` is no longer zero. That's a deliberate breaking change to the unexported behavior — `Action(0)` used to silently mean `ActionRetry`, now it means "no classification" and the switch panics on it.

### What warrants review

- The panic-on-unknown-Action at `/robust/chat_completer.go` in the action switch: `panic(fmt.Sprintf("robust: classifier returned unknown Action %d", act))`. Triggers when a user's custom classifier returns a value outside the declared enum — a programmer-error signal.
- The regex in `/robust/classify.go` with its internal test at `/robust/classify_test.go`. The test has both positive and negative cases; worth verifying the negative set covers the patterns the reader cares about.
- Span lifetime handling in `commitOnFirstPart`: check that every return path ends the attempt span exactly once, and that the success wrapper's defer ends both spans exactly once.

### Future work

Issue #210 (wrap SDK errors in gai-native types) is the real fix for the classifier. Issue #211 (iter.Pull2 goroutine-leak contract) is the real fix for the caller-must-drain requirement. Both linked from the design doc open-questions section.

## Step 5: Example and decisions

### Prompt Context

**Verbatim prompts:**
- "Add an example using this called 'robust' in internal/examples"
- "Use go run"
- `/fabrik:decisions` → "both"

**Interpretation:** Add a runnable example demonstrating the robust wrapper (primary OpenAI, fallback Anthropic), verified via `go run` rather than `go build` (the latter conflicts with the `robust` directory name when run against the package directly). Then record the two significant architectural decisions from the PR in `/docs/decisions.md`.

**Inferred intent:** Ship a complete feature — the documentation + examples + decision record that a newcomer would look for.

### What I did

Created `/internal/examples/robust/main.go` showing OpenAI primary + Anthropic secondary, `MaxAttempts: 3`, `BaseDelay: 500ms`, `MaxDelay: 5s`, debug slog handler. First draft included a custom classifier with a sentinel error for illustration — simplified it down to the no-classifier-nil-defaults case, which is the 90% use case.

Verified with `go run ./internal/examples/robust` (no keys in env, expected to fail with 401s). Output demonstrated the cascade perfectly:

```
time=2026-04-17T13:47:06.074+02:00 level=DEBUG msg="robust: falling over to next completer" from_index=0 to_index=1 error="POST \"https://api.openai.com/v1/chat/completions\": 401 Unauthorized ..."
time=2026-04-17T13:47:06.272+02:00 level=DEBUG msg="robust: all completers exhausted" final_error="POST \"https://api.anthropic.com/v1/messages\": 401 Unauthorized ..."
```

The OpenAI error string contains "401 Unauthorized" → regex extracts `401` → `ActionFallback` → Anthropic tried → also 401 → exhaustion. Exactly as designed.

Then ran `/fabrik:decisions` and appended two entries to `/docs/decisions.md`:
1. Streaming commit-on-first-part in robust wrappers — documents the alternatives (only pre-stream, mid-stream retry, commit on first) and the tradeoffs accepted.
2. SDK-agnostic default error classifier — documents why we don't import provider SDKs in the robust package and how the regex approach + planned issue #210 fit together.

### Why

The example is the smoke test: it runs without any configuration and demonstrates failover behavior via debug logs. Decisions are the record future contributors will actually read when trying to understand why the package looks the way it does.

### What worked

Running `go run ./internal/examples/robust` without API keys turned out to be a legitimate end-to-end test of the cascade — the 401s from both providers exercise the full failover path. Didn't cost any API credits.

### What didn't work

First build attempt hit the directory/output conflict:

```
$ go build ./internal/examples/robust/
go: build output "robust" already exists and is a directory
```

`go build` defaults to writing the binary to the current directory with the package name, which collided with the `robust` package directory. Switched to `go build -o /dev/null ./internal/examples/robust/` for verification, and the user followed up with "Use go run" which sidesteps the issue entirely.

### What I learned

`go build ./some/dir/` has this footgun when the binary name matches an existing directory in the working directory. `go run ./some/dir/` is the cleaner verification command for example binaries.

### What was tricky

Deciding how much to show in the example. My first draft had a custom classifier with a sentinel error, but that's gilding the lily for an introductory example — the built-in default is the 90% case. Stripped it down.

### What warrants review

- `/internal/examples/robust/main.go` — keep it minimal.
- The two entries at the bottom of `/docs/decisions.md` — matches the existing format (`## Title (YYYY-MM-DD)`, not the skill's suggested `## YYYY-MM-DD: Title`).

### Future work

Once issue #210 lands, this example could be extended to show the typed-error classifier. Not for this PR.
