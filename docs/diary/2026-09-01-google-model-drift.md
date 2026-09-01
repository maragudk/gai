# Diary: Triage new Google model IDs to unblock Dependabot client-SDK bumps

Google's live model list grew two new IDs (`gemini-3.5-transcribe`, `gemini-omni-1.1-flash`), so `TestModelConformance` in `/clients/google` fails on every CI run — including the two open Dependabot PRs bumping the client SDKs (#343 anthropic-sdk-go 1.63.1→1.66.0, #344 genai 1.68.0→1.69.0). The drift is live-API-side, unrelated to the bumps themselves. The fix lands on main first; the Dependabot PRs go green after.

## Step 1: Diagnose the CI failures and decide the triage by precedent

**Author:** main

### Prompt Context

**Verbatim prompt:** "Look at open dependabot PRs for client updates and see if we can merge them." Follow-up when asked whether to export or ignore `gemini-omni-1.1-flash`: "I think we have precedence for how to handle this? Check diary"
**Interpretation:** Review the open Dependabot client-SDK PRs, get them mergeable, and resolve the curation question by applying the established policy rather than re-litigating it.
**Inferred intent:** Keep the client dependencies current with minimal ceremony, trusting the model-constant curation policy and its diary/decision record to settle new-model triage mechanically.

### What I did

Listed open Dependabot PRs (`gh pr list --author "app/dependabot"`): #343 and #344, both with a failing `Test` job. Pulled the failed logs (`gh run view --log-failed`): both fail identically in `clients/google` — `TestModelConformance/every_listed_model_ID_is_exported_or_ignored` reports "export or ignore these model IDs: gemini-3.5-transcribe, gemini-omni-1.1-flash". Read `/clients/google/model_conformance_test.go`'s ignore list and the 2026-08-17 model-conformance diary to apply precedent: modality variants (TTS, image, robotics, music) are ignored, so `gemini-3.5-transcribe` (speech-to-text) joins that block; stable GA non-preview Gemini IDs that support `generateContent` are exported (as `gemini-3.5-flash-lite`, `gemini-3.6-flash`, `gemini-3.7-flash` were in Step 2 of that diary), so `gemini-omni-1.1-flash` gets a constant — subject to the Step 4 alive-but-unusable rule, so the builder must verify it actually works through `ChatCompleter` before exporting. Created worktree `google-model-drift` and started this diary; implementation is delegated to a builder next.

### Why

The conformance test exists precisely to redden CI on provider-side drift and make triage a mechanical policy application. Both new IDs fit existing precedent cleanly, so no new decision was needed — only verification that the omni model passes the API-surface bar.

### What worked

The failing test's own error message enumerated the exact triage backlog, and the prior diary plus the ignore list's category comments made both dispositions obvious.

### What didn't work

Nothing failed; diagnosis was three `gh` commands and two file reads.

### What I learned

The Dependabot PRs' failures looked like dependency breakage but were live-API drift that would fail on main too — the conformance test couples CI health to provider state by design, so a red Dependabot PR needs its failure read before blaming the bump.

### What was tricky

Only the omni call: `gemini-omni-flash-preview` sits in the ignore list as a preview without a stable counterpart, and `gemini-omni-1.1-flash` looks like that stable counterpart arriving — which flips the family from ignored to exported per policy, pending the live `ChatCompleter` check.

### What warrants review

After the builder's change: the ignore entry sits in the modality-variants block, the new constant follows the existing naming scheme, the omni model was live-verified through `ChatCompleter` (and its thinking-level behavior probed per the Step 3 precedent), and `TestModelConformance` is green live.

### Future work

Merge #343 and #344 once main is green; check whether the ignored `gemini-omni-flash-preview` entry should note it now has a stable exported counterpart.

## Step 2: Probe both new IDs live and ignore them both

**Author:** drift-builder

### Prompt Context

**Verbatim prompt:** "You are \"drift-builder\", a builder working in the worktree at /Users/maragubot/Developer/gai/.claude/worktrees/google-model-drift (branch worktree-google-model-drift) of the maragu.dev/gai Go library. [...] 1. **Ignore `gemini-3.5-transcribe`.** [...] 2. **Export `gemini-omni-1.1-flash`** — conditionally. [...] So first live-probe a simple chat completion against it. [...] If chat completion does NOT work through `ChatCompleter`: do not export; add it to the ignore list with a comment stating the probed rejection (quote the error), next to `gemini-omni-flash-preview`. 3. Whichever way 2 goes, consider whether the `gemini-omni-flash-preview` ignore entry's comment needs updating" (excerpted from the lead's full brief; the elided text specifies the naming scheme, the thinking-level probe protocol from Step 3 of the 2026-08-17 diary, and the validation commands)
**Interpretation:** Ignore the transcription model mechanically, then let a live probe decide the omni model's fate: export with a constant and thinking-level matrix rows if it chat-completes, ignore it with the quoted error if it doesn't.
**Inferred intent:** Discharge the drift so CI goes green on main and the two Dependabot PRs, applying the existing policy — including the alive-but-unusable bar — rather than assuming the new ID is exportable because it looks stable.

### What I did

Copied `/Users/maragubot/Developer/gai/.env.test.local` into the worktree root (`git check-ignore -v` confirms the `/.env.*.local` rule covers it; never committed) and reproduced the failure live. Wrote a throwaway probe test in the package (`clients/google/zz_omni_probe_test.go`, deleted after use) that logged the model's get-by-ID metadata and then called `ChatComplete` against `gemini-omni-1.1-flash`. The metadata looks exportable — display name "Gemini Omni 1.1 Flash", version 001, `SupportedActions=[generateContent countTokens]`, 131k/65k token limits — but the chat completion fails, so the model is not exported. Extended the probe to the non-streaming `Models.GenerateContent` path and to the sibling `gemini-omni-flash-preview`: all four combinations fail identically, so the disposition is the whole family, deterministically.

The change is eight lines in `/clients/google/model_conformance_test.go`. `gemini-3.5-transcribe` joins the modality-variants block between `gemini-3.1-flash-tts-preview` and `gemini-robotics-*`, and that block's comment gains "transcription" — the same word `/clients/openai/model_conformance_test.go` already uses for `gpt-transcribe` and `gpt-live-transcribe`. Both Omni IDs now sit in their own block, moved out of the "previews without an exported stable counterpart" block, under a comment modelled on the openai list's Responses-API-only entry: only usable via the Interactions API, rejected by the generateContent endpoint this package targets, with the verbatim 400 quoted. No constant, no `exportedModels` row, no thinking-level matrix rows, and no change to the thinking-level doc comment — none of the conditional work in requirement 2 applies.

### Why

Step 4 of the 2026-08-17 diary makes API-surface fit part of the curation bar: a constant whose only reachable behavior through `ChatCompleter` is a runtime error is a trap, and alive-but-unusable gets the same treatment as dead. `gemini-omni-1.1-flash` is exactly that shape — it resolves on get-by-ID and advertises `generateContent`, but every call to that endpoint is refused. Probing the preview too was what turned requirement 3 from a judgement call into a fact: both IDs fail for one reason, so one comment covers both, and the preview's old rationale ("preview without a stable counterpart") is not just stale but wrong — the stable counterpart has arrived and is equally unusable.

### What worked

Probing before writing any production code. The lead's brief and Step 1's triage both expected an export here, and the metadata backs that reading; only the live call disagreed. The throwaway in-package test (rather than curl) probed the exact path `ChatCompleter` takes, and widening it to two IDs × streaming/non-streaming cost one extra run and settled the family question outright.

### What didn't work

The export path, by design of the probe: `gemini-omni-1.1-flash` returns `Error 400, Message: This model only supports Interactions API., Status: INVALID_ARGUMENT, Details: []` — through `ChatComplete` (surfacing as a stream error while draining parts) and through a direct `Models.GenerateContent` call, and `gemini-omni-flash-preview` returns the byte-identical error on both paths.

Separately, `go test -shuffle on ./clients/google` has one failure unrelated to this change: `TestChatCompleter_ChatComplete/can_chat-complete_via_Vertex_AI_with_service_account` panics in `newVertexAIClientWithCredentials` because `genai.NewClient` finds no credentials — the worktree has no `vertex.json` (gitignored) and `.env.test.local` carries no `GOOGLE_VERTEX_CREDENTIALS_PATH` (only `ANTHROPIC_KEY`, `GOOGLE_KEY`, `GOOGLE_VERTEX_KEY`, `OPENAI_KEY`). Every other test in the package, the whole thinking-level matrix included, passes.

### What I learned

`SupportedActions` is a claim about the model, not a promise about the endpoint: both Omni IDs list `generateContent` and then refuse it. The conformance test's freshness filter keys on exactly that field, so a model can pass the filter, look GA in every metadata dimension, and still be unusable — the live call is the only oracle that settles it. Google now has a third API surface (Interactions) beyond generateContent and embedContent, mirroring OpenAI's Responses-only pro models; the policy's API-surface clause, written with OpenAI in mind, transferred to Gemini without amendment.

### What was tricky

Nothing technically, but the disposition inverted the brief's expected outcome, so the care went into evidence: repeating the probe across two IDs and two call shapes to rule out a transient or a key-scoping artifact (the Step 3 lesson about distinguishing "killed" from "restricted"). The error is model-specific and explicit rather than a permission error, and identical across all four probes.

### What warrants review

Judgement call worth a second opinion: the two Omni entries are exact IDs rather than a `gemini-omni-*` prefix. Exact entries mean a future Omni ID reddens CI for triage — correct if Google ever puts the family on generateContent, noise if it never does. The rest is mechanical and worth confirming: the transcribe entry's alphabetical placement, that no constant or matrix row was added, and that the preview's move out of the previews block leaves that block's comment still accurate for its remaining three entries. Validation: `go test -count=1 ./clients/google -run TestModelConformance` green live twice, once with `-shuffle on`; `golangci-lint run` reports 0 issues; `go test -shuffle on ./clients/google` green except the Vertex service-account test noted above.

### Future work

The lead's Step 1 follow-up about noting a stable exported counterpart on the preview entry is resolved and needs no separate pass. If Google later exposes the Omni family through generateContent, both ignore entries come out together and the export question reopens — including the thinking-level probing that was skipped here. The worktree's missing Vertex service-account credentials should be sorted before anyone reads a full-package run as a regression signal.
