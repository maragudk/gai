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
