# Decisions

## Finish reason normalisation (2025-02-14)

This note tracks how we normalise model finish reasons in `ChatCompleteFinishReason` and what raw signals the major providers expose.

### Normalised enum

```
unknown | stop | length | content_filter | tool_calls | refusal
```

### Provider cheat sheet

- **OpenAI**
  - `finish_reason`: `stop`, `length`, `tool_calls`, `content_filter`, plus legacy `function_call`.
  - A refusal appears via non-empty `choice.Message.Refusal` blocks while the finish reason often remains `stop`.
  - Map `function_call` to `tool_calls` when tool invocation is still surfaced through the legacy field; otherwise fold into `stop`.
- **Anthropic (Messages API)**
  - `stop_reason`: `end_turn`, `max_tokens`, `stop_sequence`, `tool_use`.
  - Claude refusals arrive as a `tool_use` content block with name `refusal` while `stop_reason` is typically `end_turn`.
- **Google Gemini (Generative Language / Vertex AI)**
  - `FinishReason`: `FinishReasonStop`, `FinishReasonMaxTokens`, `FinishReasonSafety`, `FinishReasonRecitation`, `FinishReasonOther`, `FinishReasonUnspecified`.
  - Policy blocks (`Safety`, `Recitation`) indicate moderation stops; the candidate output is empty.

### Suggested mapping

- `ChatCompleteFinishReasonStop`
  - OpenAI: `stop`, legacy `function_call` when no tool payload is present.
  - Anthropic: `end_turn`, `stop_sequence`.
  - Gemini: `FinishReasonStop`.
- `ChatCompleteFinishReasonLength`
  - OpenAI: `length`.
  - Anthropic: `max_tokens`.
  - Gemini: `FinishReasonMaxTokens`.
- `ChatCompleteFinishReasonToolCalls`
  - OpenAI: `tool_calls`, legacy `function_call` when a tool request is embedded.
  - Anthropic: `tool_use`.
  - Gemini: no direct analogue; leave as `unknown` unless future APIs surface structured tool requests.
- `ChatCompleteFinishReasonContentFilter`
  - OpenAI: `content_filter`.
  - Anthropic: none today (policy issues usually arrive as errors).
  - Gemini: `FinishReasonSafety`, `FinishReasonRecitation` if we want to preserve moderation context distinct from explicit refusals.
- `ChatCompleteFinishReasonRefusal`
  - OpenAI: any choice with a `Refusal` block regardless of `finish_reason`.
  - Anthropic: refusal `tool_use` block with `stop_reason=end_turn`.
  - Gemini: candidates blocked by safety ratings (`FinishReasonSafety`, `FinishReasonRecitation`, `FinishReasonOther`) when the model explicitly declines rather than the platform silently filtering.
- `ChatCompleteFinishReasonUnknown`
  - Catch-all for `FinishReasonUnspecified`, `FinishReasonOther`, `null`, or any vendor-specific additions we do not yet recognise.

### Implementation notes

- Always capture the raw provider value alongside the normalised enum for debugging.
- Treat explicit refusal payloads as higher priority than generic moderation finish reasons.
- Expect new enum members over time; default to `unknown` and log so we can audit additions quickly.

## Streaming commit-on-first-part in robust wrappers (2026-04-17)

The `robust` package wraps `gai.ChatCompleter` with retries and fallbacks. Streaming complicates this: `ChatCompleteResponse` yields parts via `iter.Seq2[Part, error]`, and the iterator can error after emitting partial output. We had to decide how retry and fallback interact with that timeline.

### Alternatives considered

- **Only retry on pre-stream errors.** Simple, predictable, but misses "provider accepted the request then immediately failed" cases.
- **Retry even mid-stream, discarding parts already emitted.** Maximally resilient but produces confusing user-visible output (half a sentence, then a different completion).
- **Commit on first part.** Peek the first streamed part eagerly inside `ChatComplete`; any error before the first part is treated as a pre-stream error and retries or falls back; once a part has been yielded, the stream is committed and mid-stream errors pass through unchanged.

### Decision

Go with commit-on-first-part. It catches the common "opened connection, then errored" failure mode without ever showing the caller duplicated partial output. Implemented via `iter.Pull2` to peek synchronously, returning a wrapper response that yields the buffered first part then delegates to the underlying iterator.

### Tradeoffs

- `ChatComplete` now blocks until the first token arrives, rather than returning immediately with a lazy stream. Morally equivalent to "wait for response headers" — acceptable.
- The `iter.Pull2` goroutine requires the caller to drain `Parts()`; dropping the response without iterating leaks the goroutine and the OTel spans bound to it. Documented contract on `ChatComplete`; proper fix tracked as issue #211.

## SDK-agnostic default error classifier (2026-04-17)

The `robust` package needs to classify errors from underlying completers as retry / fallback / fail. Each provider SDK (`anthropic-sdk-go`, `openai-go`, `google.golang.org/genai`) exposes HTTP status through a different concrete error type, and each stores it as a struct field rather than a method.

### Alternatives considered

- **Import all three SDKs and match concrete error types.** Precise classification, but forces every adopter of `robust` to pull in all three SDK dependencies even if they only use one. Violates the minimal-abstraction spirit of gai.
- **Introduce a gai-native error type wrapping SDK errors.** Clean long-term solution, but requires a cross-cutting change at every client boundary.
- **SDK-agnostic best-effort classifier.** Recognise `context.Canceled` / `context.DeadlineExceeded`, then fall back to a regex over the error string to extract a 4xx/5xx status. Accept the false-positive risk.

### Decision

Ship the SDK-agnostic best-effort classifier now; defer the gai-native error type. The regex is tightened to reject matches adjacent to `:`, `.`, `/`, or digits so ports, IP octets, and path segments don't trigger misclassification. Callers who need precision can supply their own `ErrorClassifierFunc`.

### Tradeoffs

- False-positive and false-negative risk from string inspection is real but bounded; our regex test covers the common failure patterns.
- The `robust` package stays a lightweight, zero-transitive-dependency wrapper.
- Tracked as issue #210: wrap provider SDK errors in a gai-native type that exposes capability interfaces (`StatusCode() int`, etc.), so classifiers can match on interfaces without importing SDKs. Once landed, the regex path can be retired.
