# Diary: QA review of thought-token streaming

**Author:** qa

QA review of task #2 on the `thought-tokens` team: a 10-file diff (+288/-11) on the `worktree-thought-tokens` branch that adds `gai.PartTypeThought`, wires up reasoning-content streaming for Google and Anthropic, removes the unconditional Anthropic `ThinkingLevel` panic, and (in a late re-add) populates `Usage.ThoughtsTokens` for OpenAI from `CompletionTokensDetails.ReasoningTokens`. The lead asked for a thorough review hitting five specific scrutiny points and structured findings with file:line refs.

## Step 1: Prime on the existing client code while task #1 was in_progress

### Prompt Context

**Verbatim prompt:** "You're the QA reviewer on team `thought-tokens`. Task #2 is yours; it's blocked by task #1 (the builder is implementing). Wait for the builder to finish — when they message you or when you see task #1 marked completed, claim task #2, run the review per its description, and report back to the lead. While you wait, you can read the existing client code (`clients/openai`, `clients/google`, `clients/anthropic`), the top-level `chat_complete.go`, and any `docs/design/*` files so you're primed for review."
**Interpretation:** Don't sit idle. Front-load the client and abstraction reading so the diff review is fast and well-grounded once the builder hands off.
**Inferred intent:** Lead wants the review turn-around tight; the priming is also a hedge against shallow comments.

### What I did

Read `chat_complete.go` end-to-end, then each client's `chat_complete.go` and `thinking_level_test.go`. Surveyed the worktree layout via `ls`. Built a mental map of the pre-diff state: gai had no `PartTypeThought`; Google's response loop branched only on `part.Text != ""`; Anthropic panicked unconditionally on any `ThinkingLevel`; OpenAI mapped `ThinkingLevel` to `ReasoningEffort` but didn't expose any reasoning content. Sent a status message to the lead summarising the pre-diff baseline.

### Why

Reading the client before the diff means I notice silent regressions instead of just rubber-stamping additions. It also lets me ground my own pass against the reviewer reports later — the parallel-reviewer pattern only helps if I can cross-check both reports independently.

### What worked

- The pre-read set me up to spot the round-trip footgun later (the request-building switches all panic on unknown part types, which means every new `PartType` carries a round-trip risk).
- Reading `clients/google/chat_complete.go` made the diff's `if part.Thought` branch instantly readable and I immediately saw `IncludeThoughts: true` was sitting inside the existing `if req.ThinkingLevel != nil` block — the lead's "is it conditional?" question was answerable in seconds.

### What didn't work

Nothing failed. The wait-and-read loop went smoothly.

### What I learned

The `Usage` struct semantics are heterogeneous across the three clients in subtle ways that aren't documented at the abstraction layer. Anthropic's SDK doesn't expose a separate thinking-token count at all (verified during review against the SDK source); OpenAI exposes it via `CompletionTokensDetails.ReasoningTokens` on Chat Completions; Google has a dedicated `ThoughtsTokenCount`. A single `ChatCompleteResponseUsage.ThoughtsTokens` field papers over those differences without acknowledging them.

### What was tricky

The lead repeatedly updated the OpenAI scope mid-review (rolled back, then re-applied). I had to keep my mental model of "what's in the diff" loose enough to absorb the changes without re-doing the whole pass.

### What warrants review

Nothing in this step — no code changes.

### Future work

None directly; the priming fed straight into the next step.

## Step 2: Run the review with two competing sub-reviewers and synthesize

### Prompt Context

**Verbatim prompt:** "Builder marked #1 completed. Full diff is now in the worktree (10 files, +288/-11) covering Google + Anthropic + gai abstraction + OpenAI token-count. Run your synthesis pass on the parallel reviewer reports plus your own checklist, and report back with a structured finding list."
**Interpretation:** Use the `fabrik:code-review` skill (two competing reviewers, intersection-of-findings synthesis), then layer my own checklist run on top, then file follow-up tasks for anything actionable.
**Inferred intent:** High-signal report. The lead doesn't want a wall of nitpicks; they want correctness/safety findings with file:line refs and a clean ship/follow-up/block verdict.

### What I did

Dispatched two sub-reviewers in parallel via the Agent tool, both with prompts that included the lead's five scrutiny points and explicit instructions to verify SDK types themselves rather than trusting the diary. While they ran, I did my own pass: `git diff` per file, manual trace of the streaming switch in each client, `go build ./...`, `go test -shuffle on ./...` (separated unit-test paths from API-keyed integration tests), `golangci-lint run ./...`, and a careful read of the new `docs/decisions.md` entry on the budget table.

When both reviewers reported back, I synthesized:

- **Consensus findings** (both reviewers + my own pass) → filed as follow-up tasks #3, #4, #6, #7. Targets: doc gap on `gai.ThinkingLevel`, `MaxCompletionTokens` silent-bump policy, dead `anthropic-sdk-go` import in the test file, Google's `ThinkingLevelNone` panic divergence.
- **Single-reviewer-but-serious** → reviewer #2 caught a real footgun: every client's request-building switch panics on `PartTypeThought`, so the natural multi-turn pattern (collect streamed parts, replay as history) breaks. Reviewer #1 missed it. I promoted it because it's a real correctness issue for callers, even though the failure mode is loud (panic, not silent drop). Filed as task #5.
- **Single-reviewer minor findings** dropped from the surfaced list: `ai.temperature` span attribute lost on Anthropic when thinking is on (observability nit, R#1 only), `RedactedThinkingBlock` silent drop (edge-case, R#1 only), Anthropic `Meta.Usage` entirely unpopulated (pre-existing, R#2 only), diary's Step 6 stale (R#2 — already known to lead).

Sent a structured report to the lead with file:line refs, verbatim test/lint output, explicit confirmation on each of the five scrutiny items, and the list of filed follow-ups. Kept task #2 in_progress pending the lead's call on tasks #3-#7.

### Why

The `fabrik:code-review` skill explicitly mandates two competing reviewers + intersection-of-findings to maximise signal-to-noise. My own pass acts as a third voice that can break ties or promote single-reviewer findings on judgment. The structured task filings give the builder concrete next steps without dumping everything into a single message.

### What worked

- **Reviewer #2's round-trip catch**. Reviewer #1 produced a clean, well-organised report but missed the round-trip panic entirely. That's exactly why two reviewers are valuable — different lenses catch different things.
- **My own SDK verification**. Both the OpenAI inline comment ("Chat Completions doesn't stream reasoning text") and the Anthropic claim ("SDK has no separate thinking-token field") are checkable via `go env GOPATH`/grep, and I (and the reviewers) did. Trust-but-verify on diary claims is non-negotiable.
- **The `<=` vs `<` math**. The bump guard `int64(maxTokens) <= thinkingBudget` looks wrong at a glance because Anthropic's constraint is "strictly less than", but it's actually the right operator: `<=` triggers the bump when `==`, which is correct because equality violates the strict-less constraint. Both reviewers and I independently arrived at this conclusion.
- **Test-failure triage**. The unit-test failures were all `401 Unauthorized` from API-keyed integration tests; the lead pre-warned me that's environmental. Filtering by `-run` against just the new tests + the abstraction tests confirmed the unit-test suite is clean.

### What didn't work

Reviewer #2 took ~7 minutes longer than reviewer #1 (≈465s vs ≈393s). I scheduled a wakeup loop while waiting and the lead got impatient before reviewer #2 returned, twice. I should have either (a) sent the status message proactively the moment reviewer #1 finished, instead of waiting another cache window, or (b) drafted the synthesis with reviewer #1 + my own pass in parallel with reviewer #2 still running. I did eventually do (b), but only after the second nudge from the lead.

The other miss: when filing follow-up tasks, I made the descriptions verbose (≈30 lines each). For minor cleanups like the dead-import task #6, that's overkill — the description is longer than the fix.

Concrete error worth recording: I tried `ls --time=ctime` to check transcript file age and got:

```
$ ls -la --time=ctime /Users/maragubot/.claude/projects/.../subagents/agent-afb6e5cb7c68169d9.jsonl
ls: unrecognized option `--time=ctime'
```

macOS BSD `ls` doesn't take that flag. Switched to `stat -f '%m %N'`, which worked.

### What I learned

The intersection-of-findings pattern works, but you have to actively promote single-reviewer findings on judgment. Strict intersection would have dropped the round-trip footgun, which is the most valuable finding in the whole review. The skill prompt's caveat ("unless the issue is serious") is doing real work; it's not a footnote.

I also learned that the `gai` abstraction's `Part` discriminator-and-untyped-fields shape (Type + private `text`/`toolCall`/`toolResult` pointers) makes adding new part types cheap *internally* but expensive at every client boundary, because the client switches don't have an exhaustive default branch — they panic. Every new `PartType` carries a round-trip footgun until each client adds an explicit case. Worth flagging as a structural pattern, not just a one-off bug.

### What was tricky

Three things:

1. **Scope churn from the lead**. OpenAI was in, then reverted, then re-added during the review. The reviewer prompts were drafted before the final state, so they over-scrutinised the OpenAI section. I had to filter their findings on what was actually in the diff vs. what they thought was in the diff. (E.g. reviewer #2's "diary contradicts the diff" finding became moot once the OpenAI re-add landed.)
2. **Promoting findings without overpromoting**. Reviewer #1 had four single-reviewer findings I dropped. The line between "this is just one reviewer's opinion" and "this is real but the other one missed it" is judgment-heavy. I tried to apply: correctness > observability > style.
3. **The diary's claim about MaxCompletionTokens**. The decisions doc says "the caller's explicit MaxCompletionTokens is still respected when it already exceeds the budget; we only fix the default-collides-with-budget case." But the code can't distinguish "default 16,384" from "explicit 500" — both pass through the same `<= budget` check. So the claim is misleading. That's not a code bug per se; it's a doc bug. Filed under task #4 as a decision-needed item rather than a hard fix.

### What warrants review

The lead should look at:

1. **Task #5** (`PartTypeThought` round-trip) — this is the most important follow-up. Decide whether the v1 fix is "skip in request-building" or "carry the Anthropic signed-thinking-block signature", because the latter changes the abstraction surface.
2. **Task #4** (Anthropic `MaxCompletionTokens` bump policy) — three options proposed (validate-and-error / log-and-bump / document only). Needs Markus's call on which fits the gai abstraction's general "loud over silent" stance.
3. **Task #3** (GoDoc on `ThinkingLevel`) — straightforward doc work but the lead specifically asked about it.

For my synthesis pass itself: the message I sent contains every file:line reference and verbatim test output the lead asked for. Spot-check by re-running `golangci-lint run ./...` and `go test -shuffle on -run "TestThought|TestPart|TestThinkingBudget|TestChatCompleter_ThinkingLevel" ./...`.

### Future work

- If task #5 lands with the "skip on the way in" fix, also add a regression test that round-trips a `Message{Parts: []Part{ThoughtPart("..."), TextPart("...")}}` through each client without panicking.
- Reviewer dispatch ergonomics: I should bake a "send a status update the moment reviewer #1 returns, regardless of #2" habit into how I drive parallel reviews. The lead got nudged twice in this session, which is one too many.
- The `Part` panic-on-unknown-type pattern is a structural footgun. Worth a separate design discussion: should request-building switches default to skip-and-warn rather than panic, given that new `PartType`s will keep arriving?

## Step 3: Verify a hand-off cleanup task and its acceptance gates

### Prompt Context

**Verbatim prompt (from builder):** "Task #3 is complete. Both GoDoc gaps closed in `chat_complete.go`: 1. `gai.ThinkingLevel`: now warns that a non-None value can cause providers to silently override caller-supplied fields, calls out the two Anthropic-side cases by name (`Temperature` dropped, `MaxCompletionTokens` bumped past the budget), and points at `docs/decisions.md` for full per-provider detail. 2. `gai.ChatCompleteResponseUsage`: replaced the bare struct with a struct-level paragraph plus per-field comments. The `ThoughtsTokens` field paragraph spells out: Google populates it from genai usage metadata, OpenAI from `CompletionTokensDetails.ReasoningTokens`, Anthropic always reports 0 because the SDK's `Usage` struct folds thinking tokens into `output_tokens`. Also flags that 0 is ambiguous."
**Interpretation:** Builder finished task #3 and #6, expects me to verify acceptance and mark them completed.
**Inferred intent:** Continue the hand-off cycle — small, focused tasks should round-trip fast.

### What I did

For task #3, ran `git diff -- chat_complete.go` to inspect the actual edits, ran `go doc maragu.dev/gai.ThinkingLevel` and `go doc maragu.dev/gai.ChatCompleteResponseUsage` to validate the rendered output, then ran `go build ./... && golangci-lint run ./... && go test -shuffle on -run 'TestPart_MarshalText|TestThoughtPart|TestDataPart|TestThinkingBudget|TestChatCompleter_ThinkingLevel' ./...` to confirm the acceptance gates. All green. Marked task #3 completed and pinged the builder with verification details.

For task #6 (dead `anthropic-sdk-go` import cleanup), the builder marked it completed before I had a chance to claim verification. I sanity-checked anyway: read the trimmed `clients/anthropic/thinking_level_test.go`, confirmed both the import (line 7 of the original) and the `var _ = anthropic.ModelClaudeSonnet4_0` (line 93) were gone, then re-ran lint + the relevant test suite. All clean. Pinged the builder.

### Why

Verifying every hand-off myself — even the trivial ones — keeps the cycle honest. Builder marked #6 completed without a hand-off; I confirmed the result rather than treating "marked completed" as a signal to move on.

### What worked

The builder's consolidation of the per-provider semantics into `ThinkingLevel`'s GoDoc with `ThoughtsTokens` redirecting there was actually cleaner than my original "duplicate the breakdown on both" suggestion. Single source of truth. I called that out in the verification message.

### What didn't work

Nothing failed.

### What I learned

When I file follow-up tasks, I tend to over-specify acceptance criteria. Task #3 had four bullets; the builder met them in a slightly different shape (consolidation rather than duplication) that was strictly better. A looser acceptance spec ("describe the per-provider semantics somewhere prominent") would have invited the same outcome with less prescription.

### What was tricky

Nothing tricky in this step.

### What warrants review

Nothing — both tasks are closed.

### Future work

None directly.

## Step 4: Long idle while the lead was away

### Prompt Context

**Verbatim prompt:** none — this was the autonomous-loop wakeup cycle, not a directed prompt.
**Interpretation:** Hold position with task #2 in_progress; the lead would return to prioritise #4/#5/#7.
**Inferred intent:** Don't burn cycles polling tightly; don't escalate prematurely; just be available when the lead returns.

### What I did

Cycled through ~16 hours of `<<autonomous-loop-dynamic>>` wakeups. Each one: `TaskList` to confirm nothing changed, then `ScheduleWakeup` again. Started at 5-minute intervals while there was active work, then backed off to hourly once it was clear the lead had stepped away. No tool spam, no repeated nudges to the lead.

### Why

Holding pattern is the right behaviour when the next move belongs to someone else. Polling more often or escalating to the lead would have been counterproductive — they'd already prioritised the wait by stepping away.

### What worked

The interval back-off (270s → 1800s → 3600s) kept me out of the cache-burn zone (300s) and didn't waste compute. The lead returned cleanly to a "no movement" status when ready.

### What didn't work

Nothing failed. The hold worked exactly as designed.

### What I learned

Patience is a feature. The harness will deliver a notification the moment the lead messages me; there's no advantage to checking more often than that.

### What was tricky

Resisting the urge to escalate when the lead had been silent for >12 hours. A naive read of "long silence" would suggest filing a check-in message, but the lead had given me a clear hand-off ("standing by" was the right state, not "are you still there?").

### What warrants review

Nothing — the loop just slept.

### Future work

If a future session needs a similar long hold, the pattern is established: 1800s default, hourly once the inbox has clearly gone quiet for hours.

## Step 5: QA review of the redesigned thinking surface (task #9)

### Prompt Context

**Verbatim prompt (lead heads-up):** "Heads up — the design got significantly tightened after the last review. Task #9 is yours, blocked by builder's task #8. Wait for builder to finish; then run the review per #9's description."

**Verbatim prompt (lead unblocked):** "Builder marked #1 completed again..." — actually #8, not #1. The lead was asking me to claim #9.

**Interpretation:** Builder is rewriting the thinking-level handling against a much tighter design. Markus and the lead decided the original cross-tier budget table was fake portability; only None and Minimal map honestly to Anthropic. Tasks #4/#5/#7 are subsumed by the redesign. Run a serial pass — no parallel reviewers.

**Inferred intent:** A focused redesign of moderate size doesn't benefit from parallel reviewers (they'd add latency without finding much extra). One careful serial pass is the right call.

### What I did

Claimed task #9, ran `git status` + `git diff --stat` (11 files modified + 3 new untracked test files + a new diary entry from the builder), then walked through the seven-point checklist serially:

1. **Anthropic** — read the diff, confirmed `ThinkingLevelNone` → `OfDisabled` and `ThinkingLevelMinimal` → `1024` budget, the rest panic. Verified `Temperature` and `MaxCompletionTokens` blocks have lost their `&& !thinkingEnabled` and auto-bump logic. Confirmed via `grep` that `thinkingBudgetForLevel` helper is deleted entirely. Inbound `PartTypeThought` is silent-drop with a comment pointing at `docs/decisions.md`.
2. **Google** — verified `ThinkingLevelNone` → `ThinkingBudget: genai.Ptr[int32](0)` (using SDK's own `genai.Ptr` rather than a file-private helper). Inbound `PartTypeThought` round-trips as `genai.Part{Text: part.Thought(), Thought: true}`. Other levels unchanged.
3. **OpenAI** — confirmed inbound `PartTypeThought` silent-drop in both user (line 89-93) and model (line 165-169) branches. ThoughtsTokens wiring preserved from earlier work.
4. **`gai.ThinkingLevel` GoDoc** — confirmed Temperature drop / MaxCompletionTokens bump language is gone. Confirmed the new tight paragraph mentions "Anthropic supports only None and Minimal" and "explicit-disable on Anthropic and Google". Ran `go doc maragu.dev/gai.ThinkingLevel` to verify rendering.
5. **`docs/decisions.md`** — `git diff` shows the old "Anthropic extended-thinking budget mapping" entry replaced (not appended). New "Anthropic extended-thinking scope: None and Minimal only (2026-04-28)" entry captures scope reduction, reasoning, the "gai does not pre-validate provider constraints" principle, and three deferred items including the multi-turn signature round-trip linked to https://github.com/maragudk/gai/issues/250.
6. **GitHub issue** — `gh issue view 250 --repo maragudk/gai` shows the issue exists, OPEN, well-titled, well-described, references docs/decisions.md.
7. **Quality gates** — `go build ./...` clean. `golangci-lint run ./...` reports `0 issues.`. `go test -shuffle on -run 'TestPart_MarshalText|TestThoughtPart|TestDataPart|TestChatCompleter_ThinkingLevel|TestChatCompleter_AcceptsInboundThoughtParts|TestThinkingBudget' ./...` all `ok`. `go test -shuffle on ./...` failures are exclusively environmental (401 Unauthorized for Anthropic/OpenAI, missing-API-key panic for Google).

All seven checklist items passed. No findings, no follow-ups. Marked tasks #9 and #2 completed. Sent a structured sign-off message to the lead with file:line references for every claim and verbatim test/lint output.

### Why

Serial review was the right call for this redesign. The diff was focused (a single design principle applied across three clients) and the lead had already articulated what to look for. Parallel reviewers would have duplicated work without adding signal — and would have re-discovered the same things I'd already verified before #2 closed.

### What worked

- Walking the checklist in order kept me honest. I didn't skip ahead to the high-stakes items (the GoDoc rewrite, the decision-doc replacement) and miss the small things (the `thinkingBudgetForLevel` helper deletion, the `temperature` block losing its guard).
- Cross-checking the helper-deletion via `grep -n 'thinkingBudgetForLevel\|thinkingBudget' clients/anthropic/*.go` returning empty was satisfying. The diff hides deletions of unused helpers; the grep made it explicit.
- The redesign cleanly closed every concern from my Step 2 synthesis: round-trip footgun (now silent-drop or round-trip per client), MaxCompletionTokens silent bump (gone — error surfaces verbatim), Google `ThinkingLevelNone` divergence (gone — both clients now explicit-disable). I called this out as "notes (not findings)" in the sign-off.

### What didn't work

Nothing failed. The redesign was strictly better than what I'd reviewed in Step 2; the only thing I had to do was confirm.

### What I learned

The right response to "we tried something, QA flagged structural issues" is sometimes a redesign rather than incremental fixes. The original design tried to be portable across providers' thinking surfaces; the redesign accepted that they're not portable and made the abstraction honest about it. The principle in the new decisions entry — "gai does not pre-validate provider constraints; API errors surface verbatim" — is a load-bearing decision that's worth more than any number of feature additions. It's the kind of thing that propagates to every future provider integration.

I also learned that the `parts_test.go` pattern (one file per client, asserting that streamed `Part` types round-trip without panicking) is a clean way to express "this is a structural property we care about". Worth replicating for other `PartType`s if the structure ever shifts.

### What was tricky

The trickiest part was distinguishing "the redesign genuinely closes the previous concerns" from "the previous concerns are still latent but the symptoms are different". I cross-walked each Step 2 finding against the new code:

- `MaxCompletionTokens` silent bump → genuinely gone; the code path no longer exists.
- Round-trip footgun → genuinely closed for Google (full round-trip), partially for Anthropic/OpenAI (silent-drop is intentional, with the deferred path tracked in issue #250).
- Google `ThinkingLevelNone` divergence → genuinely closed; both providers now send explicit-disable.

The Anthropic silent-drop on `PartTypeThought` is *not* the same problem as the original silent-drop-vs-panic question — that was about whether we crashed on round-trip. The new behaviour is intentional and documented; the *real* round-trip story (multi-turn extended thinking with tool use, signed thinking blocks) is a deeper problem that needs an API-surface change, captured in #250.

### What warrants review

The lead and Markus should look at issue #250 if/when a real caller hits the multi-turn extended-thinking-with-tool-use constraint. The fix shape is articulated but the API surface (extending `gai.Part` to carry opaque provider metadata) is non-trivial. Don't rush it.

For my sign-off itself: the message contains file:line references for every claim. Spot-check by re-running `golangci-lint run ./...`, `go doc maragu.dev/gai.ThinkingLevel`, and `gh issue view 250 --repo maragudk/gai`.

### Future work

- Watch for the first real caller hitting issue #250's multi-turn constraint. That'll force the API-surface decision.
- The `parts_test.go` pattern is worth generalising — if `PartType` grows again, every client should get a "does not panic on inbound `<NewType>`" test before the type ships.
- The decisions-entry-replaced-not-appended convention (replace stale entries on substantive redesign rather than letting a wall of contradictory historical decisions accumulate) is a good norm. Worth surfacing it explicitly if `docs/decisions.md` grows.
