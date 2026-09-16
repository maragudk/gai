# Diary: Research spike on dropping the Anthropic and Google clients

Markus is considering dropping `clients/anthropic` and `clients/google` and keeping only `clients/openai`, on the theory that the industry (including open-weight model hosting) has converged on the OpenAI API. This spike researched whether Anthropic and Google offer OpenAI-compatible endpoints good enough to replace the native clients, what the repo would lose, and whether the OpenAI API shape itself is stable enough to bet on. No code was changed. The research was delegated to three sub-agents; this entry records their findings and the conclusion.

## Step 1: Compat-endpoint research and codebase audit

**Author:** main

### Prompt Context

**Verbatim prompt:** I'm considering dropping support for Anthropic and Google clients, concentrating on the OpenAI API a lot of the industry has converged on, especially also for open weight models. Do deep research on whether Google/Anthropic provide OpenAI-compatible APIs, and what we would lose by dropping the clients.

**Interpretation:** A go/no-go research question with two halves: what the providers' OpenAI-compatible endpoints actually support (verified against live docs, not memory), and what the native client packages do that the OpenAI client does not, plus the repo blast radius of deleting them.

**Inferred intent:** Reduce maintenance burden (three SDKs, three sets of model constants, three live-test suites) without losing capabilities Markus depends on.

### What I did

Spawned two sub-agents in parallel. One (Fable) fetched the Anthropic and Gemini OpenAI-compatibility docs, OpenAI's Chat Completions vs Responses guidance, and the docs of thirteen open-weight servers and hosts, and probed the `/v1/responses` and `/v1/embeddings` paths on `api.anthropic.com` and `generativelanguage.googleapis.com/v1beta/openai/` unauthenticated. The other (Opus) did a read-only audit of `/clients/anthropic`, `/clients/google`, `/clients/openai`, `/chat_complete.go`, `/embed.go`, the examples, docs, CI, Dependabot config, and `go.mod`, and computed dependency shrinkage with `go list -deps -test`.

Midway, Markus said keeping provider-specific defaults would be nice, so I reframed the question as "which quirks can be a preset on top of the OpenAI client vs which need a native SDK".

### Why

The question can only be answered against live docs (compat layers change) and against the actual abstraction surface (what `gai.Part`, `ThinkingLevel`, embeddings, and the option structs support). Neither half is cheap to read inline, so both were delegated.

### What worked

Splitting external research from the codebase audit, and asking each agent for a feature matrix with sources. The two reports cross-checked each other: the audit found which features the native clients expose, the docs research said whether the compat endpoint has them.

### What didn't work

Nothing failed. Two things could not be verified without an API key: how the Gemini compat endpoint returns thought summaries (the response shape is undocumented), and whether `dimensions` or `task_type` are accepted on `/v1beta/openai/embeddings` (the docs show only `input` and `model`).

### What I learned

Anthropic's OpenAI-compatible layer (`/v1/chat/completions` on `api.anthropic.com`) is described in Anthropic's own docs as "primarily intended to test and compare model capabilities, and is not considered a long-term or production-ready solution". It ignores `response_format`, `strict`, `reasoning_effort`, PDF `file` parts, and `cache_control`; returns no thinking content; reports `usage.*_tokens_details` as always empty; and silently drops unsupported fields. There is no Responses endpoint and no embeddings endpoint (both 404). It has not appeared in release notes since its February 2025 launch.

Google's compat layer is much closer: streaming, tools, `response_format: json_schema`, `reasoning_effort` mapped to thinking level or budget, images, audio, context caching, embeddings, batch. It is still labelled beta and silently ignores unknown params. Gemini 3 tool calling requires echoing `tool_calls[].extra_content.google.thought_signature` or the API returns 400, which `openai-go` does not model. Vertex AI is a separate endpoint with OAuth auth, `google/`-prefixed model names, and no OpenAI-compatible embeddings at all. Multimodal embedding input exists only in the native `genai` API.

From the audit: thinking round-tripping was not implemented in any client at the time (`/clients/anthropic/chat_complete.go` and `/clients/google/chat_complete.go` hard-error on inbound thought parts, `/clients/openai/chat_complete.go` drops them), so it was not a loss. The real losses were modality (PDF on Anthropic; audio and video on Google, in chat and embeddings), the Google task-type embedding constants and formatters, and Vertex/service-account auth. The OpenAI client has `BaseURL` but no `ExtraFields`, extra-header, or `reasoning_content` machinery, so "presets" would be new infrastructure. Deleting both packages would remove 21 of 46 modules (the entire gRPC/protobuf tail arrives only via `genai`), 21 of 49 `go.mod` require lines, and about 3,900 lines of Go.

Two unrelated gaps surfaced: the OpenAI client does not forward `MaxCompletionTokens`, and the Anthropic client never populates `res.Meta` (usage goes only to the OTel span).

### What was tricky

Keeping "what the docs say" separate from "what one infers from absence". Both agents were told to flag unverified claims, and the Gemini thought-summary shape and embedding knobs stayed flagged rather than assumed.

### What warrants review

Nothing to review in code. The claims most worth spot-checking if this comes up again are the Anthropic "not production-ready" wording at https://platform.claude.com/docs/en/cli-sdks-libraries/libraries/openai-sdk and the Gemini limitations at https://ai.google.dev/gemini-api/docs/openai.

### Future work

If the Gemini compat path ever matters, a live spike with a key would settle the thought-summary shape and embedding `dimensions` question.

## Step 2: Are open-weight models moving to the Responses API?

**Author:** main

### Prompt Context

**Verbatim prompt:** What about open weight models, are they moving to the responses API? Do we need another subagent? (followed by) Yes please. And note, I think they're doing it for political reasons too, since state shifts to the server, and it'll be harder to swap providers that way.

**Interpretation:** Whether Responses is becoming the native shape for open-weight models (not just a veneer on servers), and a fair assessment of the lock-in hypothesis.

**Inferred intent:** If the OpenAI shape everyone converged on is itself being replaced, the "OpenAI-compatible only" strategy is chasing a moving target.

### What I did

Spawned a third sub-agent (Fable) with the lock-in hypothesis as an explicit question, asking it to separate what is inherently stateful in Responses from what is optional, check the Harmony format and gpt-oss docs, the chat templates and reasoning docs of the major 2026 open-weight reasoning models, the inference servers' Responses implementations and RFCs, and whether Anthropic's Messages API is becoming a second de-facto standard.

### Why

The first research pass showed most servers have added `/v1/responses`, which could be read as a migration. The distinction between "added as a shim" and "native shape" decides whether Chat Completions is a safe multi-year target.

### What worked

Asking for the lock-in assessment to distinguish evidence from opinion. The agent found the LiteLLM February 2026 incident (encrypted reasoning items cryptographically bound to the issuing org) as hard evidence, and labelled the HN and blog commentary as opinion.

### What didn't work

No Anthropic or Google roadmap statement on Responses could be found in either direction; the agent reported the absence rather than guessing.

### What I learned

Only gpt-oss is Responses-shaped natively (Harmony "is designed to mimic the OpenAI Responses API"), and it is a family of one with no successor. DeepSeek V4, Qwen3.8, Kimi K3, GLM-5, MiniMax M3, Gemma 4, Mistral Magistral, and Meta Muse all use role-based messages with a reasoning field. NVIDIA's NeMo Gym engineering note states that most open-source models are still trained on Chat Completions format.

Servers implement Responses as translation: llama.cpp converts Responses to Chat Completions internally; SGLang converts to chat messages and disabled response storage by default on 2026-09-13; Ollama, OpenRouter, and DeepSeek reject `previous_response_id`. vLLM is the only one building real state, in a separate gateway (`vllm-project/agentic-api`) explicitly outside the inference core. The push toward Responses comes from OpenAI's client tooling (Codex CLI dropped Chat Completions in February 2026), not from model design.

What did converge is the replay rule: every open-weight reasoning model requires the previous turn's reasoning sent back verbatim during tool-call turns. That is client-side state, expressible in Chat Completions.

On lock-in: server-side state is optional (`store: false` plus replay-all-items is the Open Responses spec's baseline) and the open ecosystem defaults to stateless. The lock-in that actually bites is hidden reasoning, which is endpoint-agnostic; Anthropic's thinking `signature` has the same non-portability.

Anthropic Messages is a real second standard: DeepSeek, Moonshot, Z.ai, MiniMax, OpenRouter, vLLM, llama.cpp, and Bedrock all expose `/v1/messages`. So "OpenAI-compatible only" would also cut off Anthropic-shaped hosts.

### What was tricky

Nothing in particular; the sources were consistent once the "shim vs native" question was posed directly.

### What warrants review

The conclusion that Chat Completions is the safer multi-year target for open-weight models is inferred from training-format evidence and server behaviour, not from any roadmap commitment. Worth revisiting if a second Harmony-style open-weight family ships.

### Future work

A Responses path in the OpenAI client is needed eventually for OpenAI's own models (GPT-6 Astra already requires Responses for tool calling), always with `store: false`. Not urgent for open weights.

## Step 3: Conclusion and the open PR

**Author:** main

### Prompt Context

**Verbatim prompt:** Hm. I'm using both Vertex and multi-modal embeddings. (followed by) See also open PR

**Interpretation:** The two Google features the compat endpoint cannot replace are in active use, and there is an open PR relevant to the reasoning gap.

**Inferred intent:** Close the question and connect it to in-flight work.

### What I did

Concluded that all three clients stay. Listed open PRs with `gh pr list` and inspected `#336` ("Round-trip provider thought signatures through `gai.Part`") with `gh pr view`, `gh pr checks`, and `gh run view --log-failed`.

### Why

Vertex has no compat equivalent (OAuth, prefixed model names, no compat embeddings) and multimodal embedding input is native-only, so the Google client cannot be replaced. The Anthropic compat layer was already ruled out. PR #336 is the reasoning round-trip feature the research identified as the most important gap.

### What worked

PR #336 already implements the abstraction the ecosystem research landed on independently: `gai.PartMetadata`, opaque and provider-tagged, consumed only by the same provider's request builder and ignored otherwise. Test, Evaluate, and lint are green and the branch is up to date with main.

### What didn't work

The PR's only red check is govulncheck: `Vulnerability #1: GO-2026-6348`, `Module: google.golang.org/grpc`, `Found in: google.golang.org/grpc@v1.82.1`, `Fixed in: google.golang.org/grpc@v1.83.1`. Main fails the same way (issue #365, opened 2026-09-16). grpc arrives only via `genai`, so the fix is a `go get google.golang.org/grpc@v1.83.1` on main, not a change to the PR.

### What I learned

The research reversed the premise: the compat endpoints are not a substitute for the native clients, the OpenAI API shape is less settled than "converged" suggests, and the effort belongs in reasoning round-tripping (which #336 does for Anthropic and Google) rather than client removal.

### What was tricky

`gh run view --log-failed` prints runner provisioning noise before the govulncheck output; grepping for `Vulnerability|Found in|Fixed in` isolates the useful lines.

### What warrants review

PR #336 itself, which has no reviews yet. The OpenAI client is not covered by it: it still drops thought parts and has no `reasoning`/`reasoning_content`/`reasoning_details` handling, which breaks the mandatory replay rule for DeepSeek, Kimi, Qwen, and GLM via compat hosts.

### Future work

Bump grpc on main so #365 closes and #336 goes green; merge #336; record the "keep all three clients, compat endpoints rejected" outcome as a decision; then design reasoning replay for the OpenAI client (an `openai.PartMetadata` carrying the raw reasoning payload plus a per-client field-name setting). Independently: forward `MaxCompletionTokens` in the OpenAI client and populate `res.Meta` in the Anthropic client.
