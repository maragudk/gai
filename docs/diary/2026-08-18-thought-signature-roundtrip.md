# Diary: Round-trip provider thought signatures through `gai.Part` (#250, #256)

Fix issues #250 (Anthropic signed extended-thinking blocks) and #256 (Gemini 3.x `thought_signature`) with one mechanism: a marker-interface metadata field on `gai.Part`, set by the originating client on stream-read and consumed by the same provider's request builder on the next turn. The immediate trigger was the default-test-model probe (PR #335, diary 2026-08-18-default-test-models): both Sonnet 5 and Gemini 3.5 Flash Lite are blocked as test defaults by these round-trip gaps, and Sonnet 5 turned out to stream thinking blocks unprompted, making #250 unavoidable rather than opt-in. The design decision (Markus, this session) is recorded in `/docs/decisions.md` under "2026-08-18: Carry opaque provider metadata on `gai.Part` via a marker-interface field". Defaults do NOT move in this feature; that is a follow-up.

## Step 1: Choose the metadata shape and scope the feature

**Author:** main

### Prompt Context

**Verbatim prompt:** "Fix 250+256 separate in a new PR, then we'll revisit the original ask" (Markus's answer to the direction question, after choosing "Fix #250 + #256 plumbing" as the overall direction)
**Interpretation:** Implement the thought-signature round-trip plumbing as its own PR, leaving the default-test-model switch for later.
**Inferred intent:** Unblock multi-turn tool use on current-generation Anthropic and Google models at the library level, with a deliberately designed public API rather than a quick hack, so the defaults switch (and real callers) can follow.

### What I did

Read both issues end-to-end -- they were written to be fixed together and pre-analyse two API shapes ((a) `Provider any` field, (b) sibling part types). Asked Markus to choose; he picked the third option I put alongside them: a single field of an exported marker-interface type on `gai.Part`, with small typed metadata structs defined in each client package. Recorded the decision via the decisions skill, verified `docs/index.html` nowhere documents `Part` (so no docs-site work), created worktree `thought-signature-roundtrip`, and started this diary. Implementation goes to a builder next.

### Why

The metadata shape is gai's public API surface -- the one deferred, genuinely contested choice in both issues -- so it needed the product owner, not a builder mid-implementation. Everything else in the issues is mechanical: capture on stream-read, re-emit on request-build, delete the `errThoughtRoundTripUnsupported` path.

### What worked

The issues' own "What a fix would entail" sections were effectively a pre-written spec; the only work left at requirements level was the API-shape decision and scope boundaries.

### What didn't work

Nothing failed in scoping. One assumption died earlier today and shapes this feature: #250 was believed to bite only with thinking enabled, but the probe showed Sonnet 5 streams thinking blocks unprompted -- so the Anthropic hard-error path is hit by plain multi-turn collect-and-resend flows on current models.

### What I learned

Serialization is the accepted tradeoff of the marker-interface shape: an interface-typed field does not survive naive JSON round-trips of `gai.Part`. Acceptable because the data is opaque and provider-session-specific, but worth documenting on the field.

### What was tricky

Nothing yet at this level; the tricky parts (redacted-thinking blocks, foreign-metadata pass-through semantics) are flagged in the builder brief.

### What warrants review

After the builder: the exported API surface (field name, marker interface, per-client struct names and GoDoc), that foreign/absent metadata is ignored rather than erroring, that the `errThoughtRoundTripUnsupported` path and its tests are gone, and live multi-turn tool tests green on explicitly pinned Gemini 3.x and Sonnet 5 with thinking.

### Future work

Revisit the default test models (Markus's original ask, held in PR #335's documented state) once this lands: Sonnet 5 and Gemini 3.5 Flash Lite become possible; GPT-5.6 Luna additionally needs the `ThinkingLevelNone` pinning decision. The Sonnet 5 switch will also need the `Temperature` pins dropped from anthropic subtests.

## Step 2: Implement the metadata round-trip in core and both clients

**Author:** roundtrip-builder

### Prompt Context

**Verbatim prompt:** "You are implementing the fix for issues #250 (Anthropic signed extended-thinking blocks) and #256 (Gemini 3.x `thought_signature`) — one mechanism for both. [...] One new field on `gai.Part` of an exported marker-interface type; each client package defines its own small exported metadata struct implementing it [...] Naming is yours to propose [...] A thought part WITHOUT usable anthropic metadata in history: decide explicitly between skip-silently and error, justify in the diary"
**Interpretation:** Build the marker-interface metadata field Markus chose, wire it through google and anthropic stream-read and request-build, delete the `errThoughtRoundTripUnsupported` paths, and prove the previously-400ing multi-turn flows live on pinned models.
**Inferred intent:** Make multi-turn tool use work on current-generation Gemini and Claude models at the library level, with an API deliberate enough to survive review, so the default-test-model switch can follow.

### What I did

Core: `gai.Part` gained a `Metadata` field of new marker-interface type `gai.PartMetadata` (exported marker method `PartMetadata()`, since an unexported method would bar client packages from implementing it). The field's GoDoc carries the three warnings Markus asked for: opaque, provider-session-specific, and lost on naive JSON round-trips of `Part`.

Google (`/clients/google/chat_complete.go`): new `google.PartMetadata{ThoughtSignature []byte}`. Stream-read attaches it to `PartTypeThought` and `PartTypeToolCall` parts when the genai part carries a `ThoughtSignature`. Request-build re-emits the signature on function-call parts and re-emits thought parts as `genai.Part{Thought: true}` with the signature when present; thoughts without a signature go through unsigned (live-verified accepted), and thought parts that are empty AND unsigned are skipped since an empty `genai.Part` is invalid.

Anthropic (`/clients/anthropic/chat_complete.go`): new `anthropic.PartMetadata{Signature, RedactedThinkingData string}`. Stream-read keeps yielding `ThinkingDelta`s as thought parts and, at each block's `ContentBlockStopEvent`, yields one final empty thought part carrying the block signature (the signature only arrives in the trailing `signature_delta`, after the text deltas are already yielded). A `RedactedThinkingBlock` is one empty thought part with `RedactedThinkingData`. Request-build coalesces each contiguous run of thought parts, concatenating fragment text and emitting a single `ThinkingBlockParam`/`RedactedThinkingBlockParam` when a part with usable metadata closes the run — so the natural collect-all-streamed-parts-and-resend flow reassembles blocks byte-identically to what was signed.

Both `errThoughtRoundTripUnsupported` vars, their error paths, and their two `rejects inbound PartTypeThought as deferred` tests are gone. New live tests: google multi-turn tool use pinned to `ChatCompleteModelGemini3_5FlashLite` (the exact flow that 400'd with "Function call is missing a thought_signature") plus a foreign/absent-metadata replay on the default model; anthropic multi-turn tool use with `ThinkingLevelHigh` pinned to Sonnet 4.6, the plain collect-and-resend tool flow pinned to `ChatCompleteModelClaudeSonnet5Latest` (no `Temperature` — deprecated on Claude 5), a redacted-thinking round-trip forced via Anthropic's documented magic trigger string, and a foreign/absent-metadata replay on the default model. Also one line in `/AGENTS.md` naming the concept for future agents, and the stale deferral references in test comments updated.

Self-review ran as two competing reviewer agents over the diff. Their consensus findings, all fixed: (1) a message whose parts were all dropped (e.g. only unsigned thoughts) became empty content the APIs would 400 on — both clients now skip such messages entirely, mirroring the `if len(parts) > 0` guard the openai client already had; (2) google lost signatures arriving on plain text parts (Google's docs allow the signature on "the final part of a response"), so stream-read now attaches metadata to text parts too and request-build re-emits it; (3) the core GoDoc asserted importing packages' behavior as fact — rephrased as a contract on implementations ("must ignore values they did not produce"), with the client-package pointers kept in the style the `ThinkingLevel` doc already uses; (4) a zero-value `anthropic.PartMetadata` terminator neither signed nor reset the buffer, letting unsigned text bleed into the next signed block — any metadata of the package now ends the run. Also taken from single reviewers: the misleading "because this is an interface-typed field" causal claim in the JSON caveat (`Part` marshals via `MarshalText`, so no field survives), the google subtest rename (the client replays the thought parts and ignores only the metadata — the old name claimed it ignores the parts), a wrong "subtest above" position claim, and an exported-doc sentence making the anthropic drop-entire-part behavior visible to GoDoc readers. Explicitly not taken: the pre-existing missing `span.End()` on some google error paths (out of scope, no drive-by fixes). Both full client suites re-ran green after the fixes, and every new subtest passed at least twice in final form.

### Why

The two providers' constraints have the same shape — opaque response data that must be echoed back verbatim — so one core field with per-provider structs fixes both without leaking either provider into gai's core. The anthropic empty-terminator-part representation was chosen over yielding one big thought part per block because it preserves incremental streaming (the existing behavior callers see) while still making naive part collection round-trippable.

The explicit decision Markus asked for: a thought part without usable anthropic metadata in history is **skipped silently**, not an error. Justification: the API rejects unsigned thinking blocks outright, so passing them through is not an option; unsigned fragments are structurally expected in the natural flow (they precede their signed terminator); histories recorded from google or built by hand must not explode when replayed against anthropic; and erroring would resurrect exactly the problem #250 removes. Anthropic's own docs say prior-turn thinking blocks are ignored server-side unless required, so dropping content the API would refuse is closer to provider semantics than failing the request.

### What worked

The google side worked on the first live run — both new subtests green, including the previously-impossible 3.5 Flash Lite multi-turn tool flow. Anthropic's documented `ANTHROPIC_MAGIC_STRING_TRIGGER_REDACTED_THINKING_...` trigger made the redacted path deterministically testable — it forced a redacted block on both runs, and the round-trip of the opaque payload was accepted. Sonnet 5 streamed unprompted thinking in three consecutive runs of the final test (thoughtParts=2, signature found each time), and the resend of those signed blocks was accepted — the probe's failing case, now green end to end.

### What didn't work

The first shape of the anthropic thinking+tools test failed: Sonnet 4.6 at `ThinkingLevelMedium` with a read-file tool streamed no thinking at all on run 1 (`chat_complete_test.go:522: should surface a thinking-block signature in part metadata`), passed run 2 with thoughtParts=5, then at `High` failed 2 of 3 runs the same way. Adaptive thinking skips thought entirely when the ask is trivial — "what is in readme.txt" needs no reasoning — even at high effort, and even though the same model reliably thinks for the matrix's sheep puzzle without tools. The fix was the prompt, not the level: combining the sheep puzzle with the file read ("Solve step by step: ... Then read the readme.txt file...") produced thinking + tool call in 3 of 3 runs. One shuffled openai run also failed `can use structured output` alongside the known vision 401 — same verbatim `missing_scope` 401, same restricted local key, and it passed on direct re-run, so the local key-scope issue can intermittently hit more than the vision subtest.

### What I learned

Sonnet 5's unprompted thinking was not deterministic in my runs — the same tool prompt got thoughtParts=0 in one run and thoughtParts=2 in five others — so the Sonnet 5 test asserts conditionally: if any thought parts streamed, one must carry a signature; the resend is exercised either way. Also: within one message, an anthropic thinking block's signature validates against the concatenation of its streamed deltas, confirmed live — so fragment-level streaming plus terminator-level signatures reassemble into blocks the API accepts, with no need to buffer whole thinking blocks in the client.

### What was tricky

Where to put the signature on the anthropic stream-read: it arrives only after all text deltas, so it cannot ride the first or last text fragment without either buffering the whole block (losing streaming) or duplicating text (breaking naive collection). The empty terminator part is the only shape that keeps streaming, keeps collect-and-resend lossless, and needs no new part type — at the cost that callers who filter out empty thought parts lose the signature, which the `PartMetadata` GoDoc now documents. Second, whether google should drop or pass through unsigned thought parts was settled empirically rather than by taste: 2.5 Flash accepts unsigned `thought: true` request parts, so pass-through with an empty-part guard stands.

### What warrants review

The exported names (`gai.PartMetadata` interface + `Part.Metadata` field, `google.PartMetadata`, `anthropic.PartMetadata`) are the public API surface — bikeshed here if anywhere. The skip-silently decision for unsigned thoughts on anthropic, the buffer-reset semantics (a non-thought part or any `anthropic.PartMetadata` ends a thinking run), and the new skip-empty-message guards in both clients are the behavioral judgment calls. Validate with `go test -count=1 -shuffle on ./clients/anthropic` and `go test -count=1 -shuffle on -skip '/(service_account|Embedding_2_on_Vertex)' ./clients/google` — both were run green multiple times here, google's including the `flash-lite 3.1 + high` row that flaked in the PR #335 probe.

### Future work

Unchanged from Step 1: the default-test-model switch. One observation to carry there: the openai `missing_scope` 401 hit a non-vision subtest once in this session, so treat that whole class as environmental. A pre-existing quirk surfaced by review but left alone per the no-drive-by rule: several google error paths return without `span.End()` — worth its own small fix some day.
