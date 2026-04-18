# Diary: fix flaky tool-with-args test

GitHub issue #208 reports that `TestChatCompleter_ChatComplete/can_use_a_tool_with_args` in `clients/openai/chat_complete_test.go` is flaky, failing intermittently with `chat_complete_test.go:151: unexpected message parts`. Passes on rerun without code changes. Worked on branch `fix-flaky-tool-test` off `main`.

## Root cause (from team lead analysis)

The test does two `ChatComplete` calls. First call asks "What is in the readme.txt file?" with the `read_file` tool; the model calls the tool and the test executes it. Second call sends the tool result back and expects text only. The second-turn loop handles `PartTypeText` and fatals on anything else. GPT-5 Nano occasionally decides to make another tool call rather than answering, producing a `PartTypeToolCall` that trips the `default` branch. Not a library bug — model nondeterminism.

## What I did

Added a `System` prompt to the second `ChatComplete` request in each provider test that has the same pattern:

```go
req.System = gai.Ptr("Answer the user's question in a single sentence using the tool result. Do not call any more tools.")
```

Applied to:
- `clients/openai/chat_complete_test.go` — `can use a tool with args` subtest
- `clients/anthropic/chat_complete_test.go` — `can use a tool` subtest
- `clients/google/chat_complete_test.go` — `can use a tool` subtest

## Why this approach

The task brief offered two options: add a system prompt or make the second user message more directive. System prompt kept each message block minimal and left the user message intact (still verifying "tool result round-trips correctly and the final text response arrives"). Changing the user message felt noisier — the test asserts on tokens inside the user's question.

Didn't remove the tools from the request. The whole point of the test is that a tool result round-trips with tools still available; dropping them would change what's being tested.

## Audit of Anthropic and Google

Both had the same brittle pattern: second-turn loop fatals on anything that isn't text. Even though Anthropic and Google tests use `Temperature: 0` (which OpenAI's test doesn't — GPT-5 Nano ignores temperature anyway), temperature doesn't prevent a model from deciding to issue another tool call. Applied the same fix proactively rather than waiting for those to flake.

## Verification

- `go test -shuffle on ./clients/openai/ -run TestChatCompleter_ChatComplete/can_use_a_tool_with_args -count=5` — 5/5 pass
- `go test -shuffle on ./clients/anthropic/ -run "TestChatCompleter_ChatComplete/can_use_a_tool$" -count=5` — 5/5 pass
- `go test -shuffle on ./clients/google/ -run "TestChatCompleter_ChatComplete/can_use_a_tool$" -count=5` — 5/5 pass
- `make lint` — 0 issues

## What was tricky

Nothing really — the task brief had the root cause and acceptance criteria spelled out, so the work was mechanical: apply the same three-line change in three files, then verify. The only judgment call was whether to touch the Anthropic/Google tests at all; the brief said "if they have the same brittle pattern, apply the same kind of fix," and they did.

## What warrants review

Consider whether a helper for "consume parts, assert text-only" (or a follow-up test that specifically exercises the agentic loop case — tool call in response to tool result) would be worth adding later. Out of scope for this fix.
