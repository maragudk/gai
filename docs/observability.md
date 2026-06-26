# Observability

`gai` instruments every model call with OpenTelemetry spans. This document catalogues the
spans and their attributes so you can build dashboards, write queries, and reason about traces
without reading the source.

## Telemetry model

`gai` emits **spans only**. It registers no metrics instruments and installs no exporter. The
library calls `otel.Tracer(...)`, so its spans route to whatever `TracerProvider` you set as the
global provider; you own the exporter, sampler, and backend. The clients cache their tracer when
you construct them, so install your provider before you build a client.

Tracer names:

- `maragu.dev/gai/clients/anthropic`
- `maragu.dev/gai/clients/openai`
- `maragu.dev/gai/clients/google`
- `maragu.dev/gai/robust`

Derive metrics from spans at read time. A wide span carrying token counts, latency, and model ID
answers "P99 latency by model this week" and "total completion tokens by build" from the same
data. Pre-aggregating those into counters at write time would throw away every question you had
not thought to ask. If you run a metrics-only backend that cannot derive from spans, compute the
counters you need in your own collector pipeline from these attributes.

## Spans

| Span | Kind | Emitted by |
| --- | --- | --- |
| `anthropic.chat_complete` | client | `clients/anthropic` |
| `openai.chat_complete` | client | `clients/openai` |
| `google.chat_complete` | client | `clients/google` |
| `openai.embed` | client | `clients/openai` |
| `google.embed` | client | `clients/google` |
| `robust.chat_complete` | internal | `robust` (root, wraps the attempts) |
| `robust.chat_complete_attempt` | internal | `robust` (one per try) |
| `robust.embed` | internal | `robust` (root, wraps the attempts) |
| `robust.embed_attempt` | internal | `robust` (one per try) |

Every error path records the error on the span and sets the span status to `Error` with a short
description.

A `robust` call produces a root span and one attempt span per try, each attempt parenting the
client span of the underlying provider call.

## Chat completion attributes

These ride on `anthropic.chat_complete`, `openai.chat_complete`, and `google.chat_complete`. The
**Providers** column names the clients that emit each attribute; the rest are conditional, set
only when the request carries the matching field.

| Attribute | Type | Unit | Meaning | Providers |
| --- | --- | --- | --- | --- |
| `ai.model` | string | — | Model identifier | all |
| `ai.message_count` | int | — | Number of request messages | all |
| `ai.temperature` | double | — | Sampling temperature; set only when the request specifies one | all |
| `ai.thinking_level` | string | — | Reasoning effort; set only when the request specifies one | all |
| `ai.max_completion_tokens` | int | tokens | Completion-token cap. Anthropic always emits it (default 16384); Google only when the request sets one | anthropic, google |
| `ai.tool_count` | int | — | Number of tools offered | all |
| `ai.tools` | string[] | — | Sorted tool names | all |
| `ai.tool_choice` | string | — | Forced tool-choice mode (`any` or `tool`); set only when forcing | all |
| `ai.has_system_prompt` | bool | — | Whether a system prompt was sent. The prompt text is **not** recorded | all |
| `ai.has_response_schema` | bool | — | Whether the request asked for structured output | all |
| `ai.time_to_first_token_ms` | int | ms | Latency from the streaming call to the first part yielded | all |
| `ai.prompt_tokens` | int | tokens | Input tokens, including cache-read and cache-creation tokens (gai sums Anthropic's split; OpenAI and Google already report the combined count) | all |
| `ai.completion_tokens` | int | tokens | Output tokens | all |
| `ai.cache_read_tokens` | int | tokens | Input tokens served from the provider cache; a subset of `ai.prompt_tokens` | all |
| `ai.cache_creation_tokens` | int | tokens | Input tokens written to the provider cache | anthropic |
| `ai.thoughts_tokens` | int | tokens | Reasoning tokens | openai, google |
| `ai.total_tokens` | int | tokens | Provider-reported total tokens | openai |
| `ai.finish_reason` | string | — | Provider finish reason | openai |

## Embedding attributes

These ride on `openai.embed` and `google.embed`.

| Attribute | Type | Unit | Meaning | Providers |
| --- | --- | --- | --- | --- |
| `ai.model` | string | — | Model identifier | all |
| `ai.dimensions` | int | — | Configured embedding dimensions | all |
| `ai.input_length` | int | bytes | Byte length of the input text | all |
| `ai.prompt_tokens` | int | tokens | Input tokens; set only when the provider reports usage | openai |
| `ai.total_tokens` | int | tokens | Provider-reported total tokens | openai |

## Robust wrapper attributes

The root span carries the configuration; each attempt span carries its position and outcome.

| Attribute | Type | Unit | Span | Meaning |
| --- | --- | --- | --- | --- |
| `ai.robust.completer_count` | int | — | `robust.chat_complete` | Number of chat completers in the priority list |
| `ai.robust.embedder_count` | int | — | `robust.embed` | Number of embedders in the priority list |
| `ai.robust.max_attempts` | int | — | root | Configured attempts per implementation |
| `ai.robust.base_delay_ms` | int | ms | root | Configured base backoff delay |
| `ai.robust.max_delay_ms` | int | ms | root | Configured backoff cap |
| `ai.robust.completer_index` | int | — | `robust.chat_complete_attempt` | Zero-based position of the completer tried |
| `ai.robust.embedder_index` | int | — | `robust.embed_attempt` | Zero-based position of the embedder tried |
| `ai.robust.attempt_number` | int | — | attempt | One-based attempt counter within the current implementation |
| `ai.robust.action` | string | — | attempt | Outcome of the attempt: `success`, `retry`, `fallback`, or `fail` |
| `ai.robust.attempt_timed_out` | bool | — | attempt | Set to `true` when the per-attempt timeout fired; the attempt is retried, bypassing the error classifier. Present only on timed-out attempts |

## Invariants

- `ai.cache_read_tokens` ≤ `ai.prompt_tokens` on every chat span, across all three providers.
  `ai.prompt_tokens` is normalised to include cached tokens so this holds uniformly; a test
  enforces it (`internal/oteltest.RequireCacheReadSubsetOfPromptTokens`).
- `ai.time_to_first_token_ms` fires on the first part of any kind, including a thinking block or a
  tool call, not only on the first text token.

## Content policy

Spans record shape and counts, never message content. `gai` emits `ai.has_system_prompt` rather
than the prompt text, and `ai.input_length` rather than the embedding input. User messages and
model completions never reach a span. This keeps proprietary prompts and user data out of your
telemetry pipeline by default.

## Mapping to the OpenTelemetry GenAI semantic conventions

`gai` uses an `ai.*` namespace rather than OpenTelemetry's `gen_ai.*` GenAI semantic conventions.
The conventions remain at **Development** stability as of June 2026, and OpenTelemetry has just
moved them into a dedicated [`semantic-conventions-genai`](https://github.com/open-telemetry/semantic-conventions-genai)
repository that has not yet cut a tagged release. Migrating onto an unversioned, moving target
would trade a stable contract for churn, so `gai` keeps its own names for now and will revisit
this once the conventions reach Release Candidate and tag a release.

If you query a `gen_ai.*` backend today, translate the settled core attributes in your pipeline:

| `gai` | OpenTelemetry GenAI |
| --- | --- |
| span `<provider>.chat_complete` | span `{gen_ai.operation.name} {gen_ai.request.model}` |
| `ai.model` | `gen_ai.request.model` |
| `ai.prompt_tokens` | `gen_ai.usage.input_tokens` (gai includes cache tokens; the convention splits them out) |
| `ai.completion_tokens` | `gen_ai.usage.output_tokens` |
| `ai.cache_read_tokens` | `gen_ai.usage.cache_read.input_tokens` |
| `ai.cache_creation_tokens` | `gen_ai.usage.cache_creation.input_tokens` |

The remaining `ai.*` attributes — time-to-first-token, the `ai.robust.*` family, and the boolean
shape flags — have no settled `gen_ai.*` equivalent, so they stay as they are regardless.
