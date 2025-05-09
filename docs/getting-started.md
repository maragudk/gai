# Getting Started with GAI

This guide covers the basics to get you up and running with GAI.

## Installation

Use Go modules to install GAI:

```sh
go get maragu.dev/gai
```

You will also need a client implementation for your preferred AI provider:

```sh
# For OpenAI (GPT-4, etc)
go get github.com/maragudk/gai-openai

# For Google (Gemini, etc)
go get github.com/maragudk/gai-google

# For Anthropic (Claude, etc)
go get github.com/maragudk/gai-anthropic
```

## Basic Usage

### Setting up a client

Here's how to set up a client for OpenAI models:

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
    // Create a client using your API key
    client := openai.NewClient(os.Getenv("OPENAI_API_KEY"))

    // Use the client to work with models
    // ...
}
```

### Simple chat completion

Here's a complete example using chat completion:

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
    // Create a client
    client := openai.NewClient(os.Getenv("OPENAI_API_KEY"))

    // Create a chat completion request
    request := gai.ChatCompleteRequest{
        Messages: []gai.Message{
            gai.NewUserTextMessage("What's the capital of France?"),
        },
    }

    // Send the request
    response, err := client.ChatComplete(context.Background(), request)
    if err != nil {
        log.Fatalf("Error: %v", err)
    }

    // Process the streaming response
    var output string
    for part, err := range response.Parts() {
        if err != nil {
            log.Fatalf("Error in response stream: %v", err)
        }
        output += part.Text()
    }

    fmt.Println("Model says:", output)
}
```

### Simple embedding

Here's how to create text embeddings:

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"
    "strings"

    "maragu.dev/gai"
    openai "github.com/maragudk/gai-openai"
)

func main() {
    // Create a client
    client := openai.NewClient(os.Getenv("OPENAI_API_KEY"))

    // Create an embedding request
    request := gai.EmbedRequest{
        Input: strings.NewReader("Paris is the capital of France"),
    }

    // Send the request
    response, err := client.Embed(context.Background(), request)
    if err != nil {
        log.Fatalf("Error: %v", err)
    }

    // Use the embedding vector
    fmt.Printf("Embedding vector (first 5 values): %v\n", response.Embedding[:5])
    fmt.Printf("Vector dimension: %d\n", len(response.Embedding))
}
```

## Next Steps

Once you understand the basics, dive deeper into:

- [Chat Completion](chat-completion.md) - For advanced conversation capabilities
- [Embedding](embedding.md) - For text vectorization and similarity
- [Tools](tools.md) - For extending model capabilities
- [Evaluations](evals.md) - For testing model quality
- [Examples](examples.md) - For real-world use cases