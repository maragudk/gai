# Core Concepts

GAI uses simple abstractions to work with AI models. Learn these concepts first.

## Interfaces Over Implementation

GAI defines interfaces. It separates:
- What models can do (interfaces)
- How they do it (implementations)

This lets you swap models without changing code.

## Models and Capabilities

GAI organizes models by capability:

- **ChatCompleter**: Models that have conversations
- **Embedder**: Models that create vector embeddings

A model might implement one capability or both.

## Messages and Parts

Conversations happen through messages:

- **Message**: A single unit in a conversation
- **MessageRole**: Who sent the message (`user` or `model`)
- **MessagePart**: Content within a message (text, tool calls, etc.)

## Streaming Responses

GAI handles streaming with Go iterators:

```go
// Get streaming response
response, err := client.ChatComplete(context.Background(), request)

// Process response stream
for part, err := range response.Parts() {
    if err != nil {
        // Handle error
        break
    }
    
    // Use the part
    fmt.Print(part.Text())
}
```

This makes handling chunks of text natural.

## Tools

Tools extend model capabilities:

- **Tool**: Definition of a capability
- **ToolCall**: Model's request to use a tool
- **ToolResult**: Result of executing a tool

Tools follow a specific pattern:
1. Model requests tool use
2. Your code executes the tool
3. You send result back to model

## Type Safety with Generics

GAI uses generics for type safety:

```go
// Use specific numeric types for embeddings
var client gai.Embedder[float64]
```

This:
- Prevents type errors
- Makes code clearer
- Allows optimizations

## Provider Independence

GAI abstracts away provider specifics:

- Write code against GAI interfaces 
- Use implementation libraries for providers
- Switch providers by changing a line of code

## Evaluation Framework

GAI includes evaluation tools:

- **Sample**: Input, output, expected triplet
- **Score**: Measure between 0 and 1
- **Scorer**: Function to calculate scores
- **Result**: Score and metadata

## Standard Go Patterns

GAI follows Go idioms:

- Context for cancellation and timeouts
- Error handling with Go's error pattern
- Functions return values and errors
- Uses Go generics where helpful
- Works with standard Go tooling

## Memory Handling

For handling large inputs/outputs:

- Uses io.Reader for reading data
- Streams responses via iterators
- Enables working with large content

## No Magic

GAI avoids hidden behavior:

- Clear function calls
- Explicit parameter passing
- No global state
- Composition over inheritance

## Extensibility

GAI is designed for extension:

- Create custom tools
- Implement scoring functions
- Add new evaluation methods
- Wrap with higher-level abstractions