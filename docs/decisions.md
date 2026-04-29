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

## Vertex AI service account auth via explicit credentials path (2026-04-27)

The Google client originally authenticated to both Gemini API and Vertex AI with an API key. Vertex API keys turn out to be limited: they pin requests to a fixed regional endpoint and silently ignore `GOOGLE_CLOUD_LOCATION`. Newer multi-region-only models like `gemini-embedding-2` (published only at `global`, `us`, `eu`) return 404 on that path. We needed a way to reach those models from production without dropping the existing API-key flow that Embedding 001 callers already depend on.

### Alternatives considered

- **Keep API keys, document the limitation.** Zero work, but blocks Embedding 2 on Vertex and any future multi-region-only models. Punts the problem.
- **Drop API keys for Vertex entirely; require service account auth.** Forces every Vertex caller to migrate, but unlocks all endpoints and aligns with how Google expects production Vertex traffic to authenticate.
- **Hybrid: keep API key path, add service account path.** Both work; caller picks. More surface area but smoother migration.
- **Within the SA path, use the GOOGLE_APPLICATION_CREDENTIALS env var (implicit ADC).** Standard Google convention. But ties the library to global process state and makes it awkward to construct multiple clients with different identities.
- **Within the SA path, accept an explicit credentials file path.** The library loads the JSON via `cloud.google.com/go/auth/credentials.DetectDefault`. No env var dance, and the project ID is read directly from the JSON's `project_id` field — no need for the caller to pass it separately.

### Decision

Hybrid, with an explicit credentials path. `NewClientOptions` gains optional `CredentialsPath` and `Location` fields. For the Vertex backend: when `CredentialsPath` is set, the client loads the service account JSON, infers the project ID from it, and authenticates with those credentials at the given `Location`; otherwise the existing API-key path is preserved. Gemini backend is unchanged — API key remains the right mechanism there.

### Tradeoffs

- Vertex callers wanting multi-region models must provision a GCP service account and grant `roles/aiplatform.user`. One-time setup, but a real operational lift compared to dropping in an API key.
- For AWS-hosted production workloads, the recommended path is a service account JSON delivered via existing secrets infrastructure plus `CredentialsPath` pointing at it. Workload Identity Federation is better long-term but heavier to set up; left to callers to adopt when ready.
- Inferring project ID from the JSON works for service account files (which always carry `project_id`) but not for user credentials from `gcloud auth application-default login`. Acceptable: the local-dev gcloud case can fall back to `GOOGLE_CLOUD_PROJECT` env var, and we can always reintroduce an explicit `Project` field later if needed without breaking existing callers.
- We chose an explicit path over the implicit `GOOGLE_APPLICATION_CREDENTIALS` env var so that constructing a client doesn't depend on global process state — a single call site can configure exactly which credentials it wants.
- `Location` defaults to `"global"` when empty on the credentials path. The genai SDK already falls back to global internally, but baking the default into our wrapper makes the common case (multi-region models) zero-config and surfaces the choice in our own GoDoc rather than relying on upstream behaviour. Data-residency cases (`us`, `eu`) and single-region values stay possible by setting the field explicitly.
- Per-auth-path test helpers (`newVertexAIClientWithKey`, `newVertexAIClientWithCredentials`) replaced a single helper that set both fields. The single-helper shape silently routed Vertex tests through the credentials path whenever both were available, hiding a regression in the API-key flow. Splitting forces each test to declare which path it exercises and surfaces auth-path failures in CI.

## Per-client ThinkingLevel constants (2026-04-29)

`gai.ThinkingLevel` started as a shared lowest-common-denominator enum (`Minimal/Low/Medium/High/XHigh/Max`) defined in the core package, with each client mapping the union onto whatever its API actually accepts. As provider effort enums drifted apart on the newest models — OpenAI gpt-5.x is a moving target, Gemini 3.x dropped budget-based thinking in favour of symbolic levels, Anthropic Sonnet 4.6 / Opus 4.7 introduced adaptive thinking with an `output_config.effort` enum — that shared enum stopped reflecting any single provider faithfully and silently allowed unsupported levels through to a remote 400.

### Alternatives considered

- **Keep the shared enum.** Lowest churn, but hides per-provider capability differences and forces every client to implement a fuzzy "approximate this level" mapping. PR-251 takes this shape.
- **Per-client constants of `gai.ThinkingLevel`.** The type stays in core for the universal off-switch and for `ChatCompleteRequest.ThinkingLevel *gai.ThinkingLevel`; concrete level constants move to each client package and only exist for values the targeted models actually accept. Unsupported levels panic at the boundary instead of round-tripping to the API.
- **One typed enum per provider.** Maximum type safety, but breaks the symmetry of `ChatCompleteRequest.ThinkingLevel` being a single field across all clients and makes provider-agnostic code (the `robust` wrapper, evals) much more painful to write.

### Decision

Per-client constants of the same `gai.ThinkingLevel` type. Core keeps `type ThinkingLevel string` and exactly one constant: `gai.ThinkingLevelNone` (the universal off semantic). Each client publishes the level set its newest target model speaks:

- `clients/openai`: `Minimal/Low/Medium/High/XHigh` — the union across the gpt-5.x chat-completions family (gpt-5 / 5.1 / 5.2 / 5.4* / 5.5). gpt-5.3-chat-latest is chat-tuned and accepts only `Medium`; the union still covers it. The `ChatCompleteModelGPT5_5` constant currently wraps the bare API string `"gpt-5.5"` because openai-go v3.33.0 lags the API and does not yet expose a `ChatModelGPT5_5` enum — switch to the typed constant once the SDK ships it.
- `clients/google`: `Minimal/Low/Medium/High` — the symbolic `genai.ThinkingLevel` enum used by Gemini 3.x.
- `clients/anthropic`: `Low/Medium/High/XHigh/Max` — the `output_config.effort` enum on Sonnet 4.6 / Opus 4.6 / Opus 4.7. No `Minimal`; XHigh is Opus-4.7-only by current model coverage.

Unsupported levels at the client boundary panic with `"unsupported thinking level: <value>"`. Provider-side rejections (e.g. gpt-5 has no `none`, Gemini 3 Pro rejects budget=0, Sonnet 4.6 rejects `xhigh`) surface as 400s — the spec's "let it surface" stance — so callers see real provider errors instead of silently-degraded behaviour.

### Tradeoffs

- Provider-agnostic callers (the `robust` wrapper, evals, anything that targets multiple clients with one config) lose a single canonical "Medium" constant. They must pick a per-client constant or pass `gai.ThinkingLevelNone`. Acceptable for now: the inputs that caused this redesign were concrete provider configs anyway.
- Anthropic adaptive thinking requires two SDK fields together: `Thinking.Adaptive` enables thinking, `OutputConfig.Effort` sets the level. The probe confirmed that `Effort` alone on Sonnet 4.6 returns no thinking blocks. The client always sets both for non-`None` levels, which means `OutputConfig` may already be populated by `ResponseSchema` and the Effort assignment must merge into it rather than overwrite `OutputConfig.Format`.
- The default Anthropic test model bumps to `claude-sonnet-4-6` so integration tests actually exercise the new adaptive-thinking + effort mapping. The default Google test model is held at `gemini-2.5-flash` because Gemini 3.x enforces a `thought_signature` round-trip on tool follow-ups that `gai.Part` does not yet preserve — see #256 for the deferred plumbing work and #250 for the equivalent Anthropic deferral. New 3.x mappings are exercised by a separate `TestChatCompleter_ChatComplete_Gemini3` that stays single-turn: `gemini-3-flash-preview` covers the off path (budget=0), and `gemini-3-pro-preview` covers the on path because Pro reliably streams Thought parts where Flash does not (Flash returns thought summaries only on the batch endpoint).
