# Diary: Prune stale entries from the model conformance ignore lists

Each client's `TestModelConformance` has a package-private ignore list of provider model IDs that are deliberately not exported. Entries for retired models pile up over time. The 2026-10-05 entry in `/docs/decisions.md` sets the rule: an entry is pruned once get-by-ID fails for it. While fixing #384, the drift builder spotted OpenAI entries that are past their SDK shutdown dates: `gpt-5*-chat-latest`, `gpt-5.1-codex`, `o1*`, `o3-mini*` and `o4-mini*`.

## Step 1: Scope and hand-off

**Author:** main

### Prompt Context

**Verbatim prompt:** And prune ignore-list and make PR
**Interpretation:** Remove the ignore-list entries that meet the pruning rule, in all three clients, and open a PR.
**Inferred intent:** Keep the ignore lists honest, so they hold only IDs the providers still serve.

### What I did
Created the worktree `worktree-prune-ignore-lists`, based on the #395 branch (`worktree-remove-gemini-2-0-flash`). #395 touches `/clients/google/model_conformance_test.go` and is about to merge. I copied `.env.test.local` and `vertex.json` into the worktree, then handed the work to a builder.

### Why
The pruning rule depends on live get-by-ID results, so each entry has to be checked against the real APIs rather than judged by its shutdown date alone.

### What worked
Nothing to report yet. Setup only.

### What didn't work
Nothing failed in this step.

### What I learned
Some get-by-ID lookups keep resolving after a model leaves the list endpoint. `gemini-2.0-flash` still resolved today, even though Google lists it as shut down. A shutdown date alone isn't enough to prune.

### What was tricky
Prefix entries such as `o1*` cover several IDs. One can only be pruned when every ID it matches fails get-by-ID and none of them is still listed.

### What warrants review
Every removed entry should come with live evidence that get-by-ID fails.

### Future work
None yet.

## Step 2: Probe every ignore entry against the live APIs and prune

**Author:** prune-builder

### Prompt Context

**Verbatim prompt:** You are the builder "prune-builder". Work in the worktree at /Users/maragubot/Developer/gai/.claude/worktrees/prune-ignore-lists (branch `worktree-prune-ignore-lists`). [...] Prune stale entries from the package-private ignore lists used by `TestModelConformance` in clients/anthropic, clients/google and clients/openai (the model_conformance_test.go files). Read the 2026-10-05 entry in docs/decisions.md and the comments at the top of each ignore list. They define when an entry gets pruned. As I understand it, the rule is that an entry goes once get-by-ID fails for it. Follow the documented rule exactly, and if your reading differs from mine, say so in your report. The #384 work flagged these OpenAI entries as past their SDK shutdown dates: `gpt-5*-chat-latest`, `gpt-5.1-codex`, `o1*`, `o3-mini*` and `o4-mini*`. Check every entry in all three clients, though, not just those. [...] Prefix or glob entries can only go when no listed ID matches them and get-by-ID fails for the concrete IDs they were written for. [...] Prune only the entries the rule allows.
**Interpretation:** Probe every entry in all three ignore lists with the live list and get-by-ID endpoints, and delete only the entries that are both unlisted and failing get-by-ID.
**Inferred intent:** Keep the ignore lists honest without breaking the live conformance test.

### What I did
Wrote a throwaway `zz_probe_test.go` in each client package (deleted before the commit). It dumped the full live model list and ran get-by-ID for every listed ID, plus the unlisted IDs that matter (`claude-sonnet-4-5`, `claude-haiku-4-5`, `claude-opus-4-5`, `gemini-flash-latest-high-res-exp`, `gemini-2.0-flash`) and a bogus ID per provider as a control. A small script matched every ignore pattern against the listed IDs, using the same prefix/exact semantics as `isIgnoredModel` and, for Google, the same `generateContent`/`embedContent` scope as the test.

Removed one entry, `gemini-flash-latest-high-res-exp`, from `/clients/google/model_conformance_test.go`. Its group comment said "and experimental variants of those aliases"; that was no longer true, so I trimmed it to "Floating aliases that track the newest model; the exported constants pin versions instead." Nothing else changed.

### Why
The 2026-10-05 decision says a deprecated-but-live entry is pruned once get-by-ID of the model fails. Absence from the list is not enough. Separately, an entry that matches a currently listed ID cannot go at all, because the second subtest ("every listed model ID is exported or ignored") would fail.

### What worked
The control IDs gave clean, distinguishable failures: Anthropic `404 Not Found ... not_found_error`, Google `Error 404 ... Status: NOT_FOUND`. `make conformance` passes after the removal, so the list is consistent in both directions.

### What didn't work
Nothing failed. The premise that the OpenAI entries flagged in #384 are prunable did not hold: every one of them is still in the live list and still resolves via get-by-ID. The SDK shutdown dates are in the past, but the API still serves the models.

### What I learned
- All 140 OpenAI listed IDs resolve via get-by-ID (200). All 14 Anthropic listed IDs and the unlisted aliases resolve. All probed Google IDs resolve except the one pruned.
- The comments above the deprecated groups say "Drop the entry once the model disappears from the live list", while the decision says get-by-ID failure. I followed the decision. For every deprecated-group entry the two readings give the same answer today, because each entry is either listed (OpenAI) or still resolves (`claude-sonnet-4-5`, unlisted but get-by-ID ok). I did not reword those comments.
- Only the "deprecated but still live" groups carry an explicit pruning rule. For the other groups (aliases, modality variants, legacy generations) I applied the same test, as asked: unlisted and get-by-ID failing.

### What was tricky
Entries with no listed match are not automatically prunable: `claude-sonnet-4-5` matches nothing in the list but get-by-ID returns it, which is the exact case the 2026-10-05 decision warns about. Prefix entries like `gpt-4*` and `o1*` match many listed IDs, so they stay as a whole even if some covered IDs were dead.

### What warrants review
The table below. The one removal is justified by "not listed" plus a get-by-ID 404:

`Error 404, Message: Model is not found: models/gemini-flash-latest-high-res-exp for api version v1beta, Status: NOT_FOUND, Details: []`

Re-run `make conformance` to confirm. The probe is gone, so the table is the only record of the individual lookups.

### Future work
`claude-sonnet-4-5` (end of life 2026-11-30) and the `gpt-5`, `gpt-5-mini`, `gpt-5-nano` entries (shutdown 2026-12-11) are the next candidates once get-by-ID fails. A conformance check that flags unlisted-and-failing ignore entries automatically would remove the manual pass, but that is a test-design change and out of scope here.

### Entries checked

"Listed match" is whether a currently listed ID (for Google: with `generateContent` or `embedContent`) matches the entry. "Get-by-ID" is the result for the concrete matched IDs, or for the entry itself when nothing is listed. "ok" means the lookup succeeded.

| Client | Entry | Listed match | Get-by-ID | Decision |
|---|---|---|---|---|
| anthropic | `claude-haiku-4-5-2*` | yes (`claude-haiku-4-5-20251001`) | ok | keep |
| anthropic | `claude-opus-4-5-2*` | yes (`claude-opus-4-5-20251101`) | ok | keep |
| anthropic | `claude-sonnet-4-5-2*` | yes (`claude-sonnet-4-5-20250929`) | ok | keep |
| anthropic | `claude-sonnet-4-5` | no | ok (resolves while unlisted) | keep |
| google | `gemini-flash-latest` | yes | ok | keep |
| google | `gemini-flash-latest-high-res-exp` | no | error: `Error 404, Message: Model is not found: models/gemini-flash-latest-high-res-exp for api version v1beta, Status: NOT_FOUND, Details: []` | **prune** |
| google | `gemini-flash-lite-latest` | yes | ok | keep |
| google | `gemini-pro-latest` | yes | ok | keep |
| google | `gemini-3.1-flash-lite-preview` | yes | ok | keep |
| google | `gemini-3.1-pro-preview-customtools` | yes | ok | keep |
| google | `gemini-embedding-2-preview` | yes | ok | keep |
| google | `gemini-omni-1.1-flash` | yes | ok | keep |
| google | `gemini-omni-flash-preview` | yes | ok | keep |
| google | `gemini-2.5-computer-use-*` | yes (1 ID) | ok | keep |
| google | `gemini-2.5-flash-image*` | yes (1 ID) | ok | keep |
| google | `gemini-2.5-flash-preview-tts` | yes | ok | keep |
| google | `gemini-2.5-pro-preview-tts` | yes | ok | keep |
| google | `gemini-3-pro-image*` | yes (2 IDs) | ok | keep |
| google | `gemini-3.1-flash-image*` | yes (2 IDs) | ok | keep |
| google | `gemini-3.1-flash-lite-image*` | yes (1 ID) | ok | keep |
| google | `gemini-3.1-flash-tts-preview` | yes | ok | keep |
| google | `gemini-3.5-transcribe` | yes | ok | keep |
| google | `gemini-3.8-flash-lite-tts` | yes | ok | keep |
| google | `gemini-3.8-flash-tts` | yes | ok | keep |
| google | `gemini-nano-banana-*` | yes (1 ID) | ok | keep |
| google | `gemini-robotics-*` | yes (1 ID) | ok | keep |
| google | `lyria-*` | yes (3 IDs) | ok | keep |
| google | `nano-banana-*` | yes (1 ID) | ok | keep |
| google | `antigravity-*` | yes (3 IDs) | ok | keep |
| google | `deep-research-*` | yes (3 IDs) | ok | keep |
| google | `gemma-*` | yes (2 IDs) | ok | keep |
| openai | `gpt-5-2*` | yes (1 ID) | ok | keep |
| openai | `gpt-5-mini-2*` | yes (1 ID) | ok | keep |
| openai | `gpt-5-nano-2*` | yes (1 ID) | ok | keep |
| openai | `gpt-5.1-2*` | yes (1 ID) | ok | keep |
| openai | `gpt-5.2-2*` | yes (1 ID) | ok | keep |
| openai | `gpt-5.4-2*` | yes (1 ID) | ok | keep |
| openai | `gpt-5.4-mini-2*` | yes (1 ID) | ok | keep |
| openai | `gpt-5.4-nano-2*` | yes (1 ID) | ok | keep |
| openai | `gpt-5.5-2*` | yes (1 ID) | ok | keep |
| openai | `gpt-5` | yes | ok | keep |
| openai | `gpt-5-mini` | yes | ok | keep |
| openai | `gpt-5-nano` | yes | ok | keep |
| openai | `gpt-5.1` | yes | ok | keep |
| openai | `gpt-5.4-nano` | yes | ok | keep |
| openai | `chat-latest` | yes | ok | keep |
| openai | `gpt-5-chat-latest` | yes | ok | keep |
| openai | `gpt-5.1-chat-latest` | yes | ok | keep |
| openai | `gpt-5.2-chat-latest` | yes | ok | keep |
| openai | `gpt-5.3-chat-latest` | yes | ok | keep |
| openai | `gpt-5-pro*` | yes (2 IDs) | ok | keep |
| openai | `gpt-5.2-pro*` | yes (2 IDs) | ok | keep |
| openai | `gpt-5.4-pro*` | yes (2 IDs) | ok | keep |
| openai | `gpt-5.5-pro*` | yes (2 IDs) | ok | keep |
| openai | `gpt-5-codex` | yes | ok | keep |
| openai | `gpt-5.1-codex*` | yes (`gpt-5.1-codex`, `-max`, `-mini`) | ok | keep |
| openai | `gpt-5.2-codex` | yes | ok | keep |
| openai | `gpt-5.3-codex` | yes | ok | keep |
| openai | `gpt-5-search-api*` | yes (2 IDs) | ok | keep |
| openai | `chatgpt-image-latest` | yes | ok | keep |
| openai | `gpt-audio*` | yes (6 IDs) | ok | keep |
| openai | `gpt-image*` | yes (9 IDs) | ok | keep |
| openai | `gpt-live-1` | yes | ok | keep |
| openai | `gpt-live-transcribe` | yes | ok | keep |
| openai | `gpt-realtime*` | yes (10 IDs) | ok | keep |
| openai | `gpt-transcribe` | yes | ok | keep |
| openai | `omni-moderation-*` | yes (2 IDs) | ok | keep |
| openai | `sora-*` | yes (2 IDs) | ok | keep |
| openai | `tts-1*` | yes (4 IDs) | ok | keep |
| openai | `whisper-*` | yes (1 ID) | ok | keep |
| openai | `babbage-002` | yes | ok | keep |
| openai | `davinci-002` | yes | ok | keep |
| openai | `gpt-3.5-*` | yes (6 IDs) | ok | keep |
| openai | `gpt-4*` | yes (28 IDs) | ok | keep |
| openai | `o1*` | yes (`o1`, `o1-2024-12-17`, `o1-pro`, `o1-pro-2025-03-19`) | ok | keep |
| openai | `o3*` | yes (8 IDs, including `o3-mini`, `o3-mini-2025-01-31`) | ok | keep |
| openai | `o4-mini*` | yes (4 IDs) | ok | keep |
| openai | `text-embedding-ada-002` | yes | ok | keep |

Controls: `claude-bogus-1` returns `404 Not Found ... "message":"model: claude-bogus-1"`, `gemini-bogus-1` returns `Error 404, Message: Model is not found: models/gemini-bogus-1 ...`. Also noted: `gemini-2.0-flash` (removed in #395) still resolves, as step 1 observed.

### Verification
`make conformance` passes (all clients). `go test -shuffle on ./clients/...` passes. `golangci-lint run` reports 0 issues.
