# Chat Completion

Chat completion is the core of interacting with language models. GAI makes it simple.

## Basic Concepts

Chat completion involves:

1. Creating a request with messages
2. Sending the request to a model 
3. Receiving a streaming response
4. Processing the response parts

## Creating a Request

A chat completion request consists of:

```go
request := gai.ChatCompleteRequest{
    Messages: []gai.Message{
        // Messages go here
    },
    System: gai.Ptr("Optional system instructions"),
    Temperature: gai.Ptr(gai.Temperature(0.7)),
    Tools: []gai.Tool{
        // Optional tools go here
    },
}
```

### Messages

Messages are the core of the conversation. Each message has:

- A role (`user` or `model`)
- One or more message parts

The simplest way to create messages:

```go
// User message with text
userMessage := gai.NewUserTextMessage("What's the capital of France?")

// Model message with text
modelMessage := gai.NewModelTextMessage("The capital of France is Paris.")
```

For a conversation, stack multiple messages:

```go
messages := []gai.Message{
    gai.NewUserTextMessage("What's the capital of France?"),
    gai.NewModelTextMessage("The capital of France is Paris."),
    gai.NewUserTextMessage("And what's the capital of Italy?"),
}
```

### System Instructions

System instructions guide model behavior:

```go
request := gai.ChatCompleteRequest{
    Messages: []gai.Message{
        gai.NewUserTextMessage("Hello"),
    },
    System: gai.Ptr("You are a helpful assistant who speaks like a pirate."),
}
```

### Temperature

Control randomness with temperature:

```go
request := gai.ChatCompleteRequest{
    Messages: []gai.Message{
        gai.NewUserTextMessage("Write a poem about Go programming"),
    },
    Temperature: gai.Ptr(gai.Temperature(0.7)), // Higher = more creative
}
```

Values range from 0.0 (deterministic) to 1.0+ (creative).

## Sending a Request

Sending a request is straightforward:

```go
response, err := client.ChatComplete(context.Background(), request)
if err != nil {
    // Handle error
}
```

## Processing Responses

Responses stream in parts:

```go
var output string
for part, err := range response.Parts() {
    if err != nil {
        // Handle error
        break
    }
    output += part.Text()
    
    // Process part incrementally if needed
    fmt.Print(part.Text()) // Stream to console
}
```

## Advanced Message Handling

### Tool Calls and Results

When using tools, handle tool calls and results:

```go
// Handle a tool call from the model
toolCall := messagePart.ToolCall()
fmt.Printf("Model called tool: %s with args: %s\n", toolCall.Name, string(toolCall.Args))

// After executing the tool, send the result back
toolResult := gai.ToolResult{
    ID:      toolCall.ID,
    Content: "Tool result goes here",
    // Set Err if there was an error
}
toolResultMessage := gai.NewUserToolResultMessage(toolResult)
```

## Complete Example

Here's a complete example with conversation history:

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"

    "maragu.dev/gai"
    openai "github.com/maragudk/gai-openai"
)

func main() {
    client := openai.NewClient(os.Getenv("OPENAI_API_KEY"))

    // Create conversation with history
    request := gai.ChatCompleteRequest{
        Messages: []gai.Message{
            gai.NewUserTextMessage("What's the capital of France?"),
            gai.NewModelTextMessage("The capital of France is Paris."),
            gai.NewUserTextMessage("What's the population?"),
        },
        Temperature: gai.Ptr(gai.Temperature(0.3)),
    }

    // Send request
    response, err := client.ChatComplete(context.Background(), request)
    if err != nil {
        log.Fatalf("Error: %v", err)
    }

    // Process the streaming response
    var output string
    for part, err := range response.Parts() {
        if err != nil {
            log.Fatalf("Error in response: %v", err)
        }
        output += part.Text()
        fmt.Print(part.Text()) // Stream to console
    }

    // Save for next conversation
    newHistory := append(request.Messages, gai.NewModelTextMessage(output))
}
```