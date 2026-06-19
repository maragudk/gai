# Diary: robust per-attempt timeout (issue #290)

Add an optional per-attempt timeout to the `robust` wrappers (`ChatCompleter` and
`Embedder`) so a *hung* backend — slow or stuck rather than erroring — can be bounded,
retried, and fallen over from. Today the only knob is a global deadline, which the default
classifier treats as fatal, defeating the retry-and-fallover the wrappers exist to provide.

## Step 1: Shape requirements and set up the feature

**Author:** main (lead)

### Prompt Context

**Verbatim prompt:** "Let's look at issue 290"
**Interpretation:** Read issue #290, understand the `robust` package, refine the proposal
into concrete requirements, and hand a builder a clear, scoped task.
**Inferred intent:** Turn a well-written but decision-bearing issue into an unambiguous
spec a builder can implement without guessing, resolving the open behavioural questions up
front.

### What I did
Read issue #290, the `robust` package (`chat_completer.go`, `embedder.go`, `classify.go`,
`backoff.go`), and `/docs/design/robust.md`. Resolved the three open product decisions with
the user, then created a worktree and started this diary.

Decisions locked:
1. **Timeout action = retry then fall over.** A per-attempt timeout maps to `ActionRetry`:
   retry the same backend up to `MaxAttempts`, then fall over to the next.
2. **Keep backoff.** The normal jittered backoff still runs between timeout-driven retries —
   no special-casing in the loop; returning `ActionRetry` is enough.
3. **Handle out of band (lead's call).** `tryOnce` detects the synthetic per-attempt
   deadline directly and translates it to `ActionRetry`, bypassing the classifier. The
   classifier never sees the synthetic timeout. Caller cancellation/deadline still flows
   through the classifier as `ActionFail` (the hard stop). Rationale: per-attempt timeout
   and caller cancellation both surface as `context.DeadlineExceeded`; only by checking the
   *parent* context's liveness can they be told apart, and the classifier never sees the
   context. Out-of-band is the only reliable place, and it gives custom classifiers
   timeout-driven retry for free.

### Why
The issue offered two approaches for the classifier question and left the retry-vs-fallover
and backoff semantics ambiguous. Pinning these down before implementation avoids a builder
guessing and avoids rework.

### What worked
The issue is detailed and self-consistent; most of the API shape (the `AttemptTimeout
time.Duration` option, zero = disabled, symmetric across both wrappers) needed no debate.

### What didn't work
Nothing yet — no code written.

### What I learned
The `ChatCompleter` commit-on-first-part design is the sharp edge here. After the first
streamed part arrives, `ChatComplete` returns a wrapped iterator that keeps reading from the
attempt's context for the rest of the stream. A naive `context.WithTimeout` + `defer cancel`
would either cancel a healthy long stream mid-flight (if the timer keeps running) or leak the
timer. The per-attempt timeout must therefore bound **time-to-first-part only** and be
stopped at commit, with the attempt context kept alive — and cancelled exactly once — for the
remaining stream. The `Embedder` has no streaming and is straightforward.

### What was tricky
Distinguishing the synthetic per-attempt deadline from the caller's own cancellation, and
the streaming-context lifetime for `ChatCompleter` (see above).

### What warrants review
N/A for this step. The builder's steps should document the timer-stop-on-commit mechanics
and the timeout-detection predicate, which are where correctness lives.

### Future work
None beyond the issue's scope. Provider-specific timeout classification stays out of scope
(the default classifier remains SDK-agnostic).

## Step 2: Implement the per-attempt timeout in both wrappers

**Author:** builder-attempt-timeout

### Prompt Context

**Verbatim prompt:** "You are implementing issue #290 in the `maragu.dev/gai` repo. [...] Add an
optional per-attempt timeout to the `robust` wrappers so a *hung* backend (slow/stuck rather
than erroring) can be bounded, retried, and fallen over from."
**Interpretation:** Add `AttemptTimeout time.Duration` to both options structs, wrap each
attempt's context when set, detect a fired per-attempt timeout out of band and return
`ActionRetry` (bypassing the classifier), keep the caller's own deadline fatal, preserve
backoff, and — critically — bound only time-to-first-part for the streaming `ChatCompleter` so
a healthy long stream isn't killed.
**Inferred intent:** Make retry-and-fallover robust against a stuck upstream, the one failure
the wrapper couldn't previously handle, without changing zero-timeout behaviour by a byte.

### What I did
Added `AttemptTimeout` to `NewEmbedderOptions[T]` and `NewChatCompleterOptions`, stored it on
both structs, and added a negative-value panic to each constructor (updating the GoDoc panic
lists). Both `/robust/embedder.go` and `/robust/chat_completer.go` changed.

`Embedder.tryOnce` is the simple case: when `attemptTimeout > 0`, wrap with
`context.WithTimeout(ctx, attemptTimeout)`, `defer cancel()`, run `Embed` against the
sub-context, and after an error check `attemptCtx.Err() == context.DeadlineExceeded &&
ctx.Err() == nil` to detect our synthetic deadline. On that path: set
`ai.robust.action == "retry"` and `ai.robust.attempt_timed_out == true`, record the error,
set the span status, and return `ActionRetry` without calling the classifier.

`ChatCompleter.tryOnce` is the sharp case. It commits on the first streamed part and then
keeps reading from the attempt context for the rest of the stream, so a `context.WithTimeout`
timer left running past commit would cancel a healthy long stream. I used
`context.WithCancel(ctx)` plus `time.AfterFunc(attemptTimeout, …)` that stores into an
`atomic.Bool` (`timerFired`) and cancels the sub-context. Detection keys off
`timerFired.Load() && ctx.Err() == nil`, not `errors.Is`, because the AfterFunc cancels with
`context.Canceled`, not `context.DeadlineExceeded`. I split the cleanup into two functions:
`stopTimer` (halts the time-to-first-part clock) and `cancel` (releases the sub-context).
`commitOnFirstPart` now takes both: on commit it calls `stopTimer()` immediately and folds
`cancel()` into the wrapped iterator's existing `defer` (`stop()` + `cancel()` +
`attemptSpan.End()` + `rootSpan.End()`), so the sub-context stays valid for the rest of the
stream and is cancelled exactly once at stream end. On the failure paths `tryOnce` calls
`stopTimer()` then `cancel()` before classifying.

Tests: added `hang`/`hangBeforeStream`/`hangInStream`/`partDelay` to the queue-driven fakes
(both now read the context), and subtests in `/robust/embedder_test.go` and
`/robust/chat_completer_test.go` covering hung-then-fallover, exhaust-MaxAttempts-then-fallover,
caller-deadline-stays-fatal, classifier-not-invoked-on-timeout, the timeout-path span
attributes, the negative-`AttemptTimeout` panic, and the streaming regression. Docs: updated
`/docs/design/robust.md` (options blocks, constructor panic list, a new "Per-attempt timeout"
subsection, the `ai.robust.attempt_timed_out` attribute, the testing summary, and a stale
`spans_test.go` reference). No `/robust` README exists and `/docs/index.html` doesn't describe
the robust options, so neither needed touching.

### Why
The out-of-band detection is the issue's central decision: a per-attempt timeout and a caller
cancellation both surface as `context.DeadlineExceeded`, and the classifier never sees the
context, so the only reliable place to tell them apart is `tryOnce`, by checking the parent
context's liveness against the sub-context's own state. Bypassing the classifier also gives
custom classifiers timeout-driven retry for free.

### What worked
The Embedder side worked first try. The loop needed no changes at all — returning
`ActionRetry` from `tryOnce` is sufficient for backoff + retry + fallover, exactly as the lead
predicted. All pre-existing tests stayed green with zero `AttemptTimeout`, confirming the
no-op path is byte-for-byte unchanged.

### What didn't work
My first `ChatCompleter` cut deferred the *entire* cleanup (timer.Stop + cancel) to the
wrapped iterator's stream-end defer instead of stopping the timer at commit. The streaming
regression test caught it immediately:

```
--- FAIL: TestChatCompleter_ChatComplete/does_not_kill_a_healthy_backend_that_delivers_the_first_part_quickly_then_streams_slowly_past_AttemptTimeout
    chat_completer_test.go:660: Expected nil error, but got "context canceled"
```

The timer stayed armed after commit and fired mid-stream, cancelling a healthy stream — the
exact failure the brief warned about. Fix: split `stopTimer` from `cancel`, stop the timer at
the commit point inside `commitOnFirstPart`, and transfer only `cancel` to the stream-end
defer.

I also briefly wrote the first version of the slow-stream fake with `partDelay` applied to the
first part too, so time-to-first-part exceeded the timeout and the timer fired correctly —
that was a test bug, not a code bug. I made `partDelay` skip the first part (`i > 0`) so the
fake models "commit fast, then stream slowly".

### What I learned
The timer-fired flag, not `errors.Is(err, context.DeadlineExceeded)`, is the authoritative
signal — and it has to be, because `context.WithCancel` + manual cancel surfaces as
`context.Canceled`. A naive `errors.Is(..., DeadlineExceeded)` predicate would miss the
`ChatCompleter` timeout entirely. The `Embedder` uses `context.WithTimeout` so its error *is*
`DeadlineExceeded`, but I kept the sub-context-state check (`attemptCtx.Err() == ...`) rather
than inspecting the returned error, for the same precision the brief asked for.

### What was tricky
The commit boundary in `commitOnFirstPart`. The success path must stop the timer but keep the
context alive; the failure paths (empty stream, error-before-first-part) must release both.
Routing `stopTimer`/`cancel` so each runs exactly once across both paths, with `cancel`
transferred to the iterator on success only, was the delicate part. The `timer.Stop()`-returns-
false race at commit is handled by simply committing anyway: a returned first part is valid per
the commit-on-first-part contract, and the rest of the stream may error through.

### What warrants review
The two functions in `ChatCompleter.tryOnce` and the commit-time `stopTimer()` call in
`commitOnFirstPart` — that's where correctness lives. Validate with `go test -shuffle on -race
./robust/`, especially the streaming regression subtest ("does not kill a healthy backend …")
and the caller-deadline-stays-fatal subtest. The detection predicates
(`timerFired.Load() && ctx.Err() == nil` for chat; `attemptCtx.Err() == DeadlineExceeded &&
ctx.Err() == nil` for embed) are the other place to scrutinise.

### Future work
None beyond the issue's scope. If issue #210's gai-native error type lands, the same
out-of-band detection could fold into a richer signal, but the current predicate is correct and
self-contained.

## Step 3: Self-review and fix a dropped-telemetry bug

**Author:** builder-attempt-timeout

### Prompt Context

**Verbatim prompt:** "When implementation is done, self-review your own diff with the
`fabrik:code-review` skill before reporting back. Fix what you find."
**Interpretation:** Run the competing-reviewers code review on the diff, then fix any real
issues it surfaces.
**Inferred intent:** Catch correctness and observability defects before handing back, rather
than shipping a plausible-looking but subtly wrong change.

### What I did
Ran the `fabrik:code-review` skill: two independent reviewer agents on the working-tree diff.
Both independently found the same real bug (strong consensus) and agreed everything else —
concurrency, cancel/timer lifetime, the detection predicates, zero-timeout preservation — was
sound.

The bug: on the in-stream-stall timeout path (`hangInStream` — `ChatComplete` returns no error
but the first iterator pull blocks until the per-attempt timer cancels `attemptCtx`),
`commitOnFirstPart` ended the attempt span itself on its `firstErr` branch and returned the
error. `tryOnce` then set `ai.robust.action == "retry"` and `ai.robust.attempt_timed_out ==
true` on the **already-ended** span, and the OTel SDK silently dropped both attributes. So the
feature's headline observability signal was invisible for exactly one of the two timeout paths
— and it's the time-to-first-part path the feature exists to bound. The retry/fallover control
flow was still correct; only telemetry was lost. Root cause was a pre-existing double-
`attemptSpan.End()` (`commitOnFirstPart`'s failure branches ended the span, and the `tryOnce`
tail ended it again) that this change newly routed design-mandated attributes through.

Fix: removed the two `attemptSpan.End()` calls from `commitOnFirstPart`'s failure branches
(empty-stream and first-error), leaving the single tail in `tryOnce` to own the span on every
failure path. The `stop()` (iterator-pull cleanup) stays; only the span End moved. Updated the
function's doc comment to state the caller owns the span on failure and why
`commitOnFirstPart` must not end it there.

I also added a regression test ("records a retry action and attempt_timed_out when the stream
stalls before the first part") that drives the `hangInStream` path and asserts the two
attributes, and renamed the existing pre-stream test to "...when the call hangs before the
stream" so the two timeout paths are clearly distinguished. Finally I widened the streaming
regression test's timing margin from 2ms/5ms to 20ms/50ms (timeout/part-delay) after both
reviewers flagged the tight 2ms commit window as a potential CI flake.

### Why
The dropped attributes broke an explicit requirement in the brief — `ai.robust.attempt_timed_out`
must be set on the timeout path. Ending the span in only one place is also simply more correct:
a span should be ended once, by whoever owns its full lifecycle, which on the failure path is
`tryOnce` (it sets the action and records the error after `commitOnFirstPart` returns).

### What worked
The competing-reviewers method paid for itself: both agents converged on the one real defect
and empirically confirmed it with throwaway recorder tests, which gave high confidence it was
real and not a false positive. I reproduced the catch by temporarily reintroducing the
`attemptSpan.End()` and watching the new test fail (`chat_completer_test.go:821: Not true`),
then restored the fix and watched it pass — confirming the test is a genuine regression guard.

### What didn't work
Nothing new broke during the fix. The fix is a deletion of two lines plus a doc-comment update;
the full suite stayed green under `-race -count=5 -shuffle on`.

### What I learned
OpenTelemetry spans are read-only after `End()` and the SDK drops post-End mutations silently —
no panic, no log. A double-`End()` is therefore invisible until you actually try to set
something between the two Ends, which is exactly what this change did. The lesson: end a span in
exactly one place, owned by the code that writes the last attribute.

### What was tricky
Confirming the fix didn't strand a span unended on any failure path. I traced every
`commitOnFirstPart` return: the two failure branches now return without ending the span, and
`tryOnce` ends it once in both the timeout branch and the classifier branch. `grep -n
"attemptSpan.End()"` shows three sites — timeout path, classifier-failure path, and the
success-path iterator defer — each reached once per attempt.

### What warrants review
The single-End invariant in `tryOnce` + `commitOnFirstPart`. Validate by reading the three
`attemptSpan.End()` sites and confirming each failure path reaches exactly one. The two span-
attribute regression tests (pre-stream hang and in-stream stall) lock the behaviour in.

### Future work
None. One reviewer noted the embedder's predicate (`attemptCtx.Err() == DeadlineExceeded`)
could, in a sub-microsecond race, treat a genuine error arriving exactly at the deadline as a
timeout-retry; both reviewers judged this benign (the outcome is still retry-then-fallover, the
safe direction) and it matches the predicate the brief specified, so I left it.
