# Examples

Real-world applications of GAI. These examples show how to use GAI effectively.

## Simple Chatbot

Create a command-line chatbot:

```go
package main

import (
    "bufio"
    "context"
    "fmt"
    "os"
    "strings"

    "maragu.dev/gai"
    openai "github.com/maragudk/gai-openai"
)

func main() {
    // Initialize client
    client := openai.NewClient(os.Getenv("OPENAI_API_KEY"))

    // Create conversation history
    var messages []gai.Message

    // Add system prompt
    system := "You are a helpful assistant. Be concise."
    
    // Start conversation loop
    scanner := bufio.NewScanner(os.Stdin)
    fmt.Println("Chat with the AI (type 'exit' to quit):")
    
    for {
        // Get user input
        fmt.Print("> ")
        if !scanner.Scan() {
            break
        }
        userInput := scanner.Text()
        if userInput == "exit" {
            break
        }
        
        // Add user message
        messages = append(messages, gai.NewUserTextMessage(userInput))
        
        // Create request
        request := gai.ChatCompleteRequest{
            Messages:    messages,
            System:      gai.Ptr(system),
            Temperature: gai.Ptr(gai.Temperature(0.7)),
        }
        
        // Get response
        fmt.Print("AI: ")
        response, err := client.ChatComplete(context.Background(), request)
        if err != nil {
            fmt.Printf("Error: %v\n", err)
            continue
        }
        
        // Process response
        var fullResponse string
        for part, err := range response.Parts() {
            if err != nil {
                fmt.Printf("\nError: %v\n", err)
                break
            }
            text := part.Text()
            fullResponse += text
            fmt.Print(text) // Stream response
        }
        fmt.Println()
        
        // Add AI response to conversation history
        messages = append(messages, gai.NewModelTextMessage(fullResponse))
    }
}
```

## Semantic Search

Build a simple semantic search engine:

```go
package main

import (
    "context"
    "fmt"
    "os"
    "sort"
    "strings"

    "maragu.dev/gai"
    "maragu.dev/gai/eval"
    openai "github.com/maragudk/gai-openai"
)

type Document struct {
    Title     string
    Content   string
    Embedding []float64
}

func main() {
    // Initialize client
    client := openai.NewClient(os.Getenv("OPENAI_API_KEY"))
    
    // Sample documents
    documents := []Document{
        {Title: "Go Interfaces", Content: "Go interfaces allow you to define behavior..."},
        {Title: "Error Handling", Content: "In Go, errors are values. Error handling..."},
        {Title: "Concurrency", Content: "Go provides goroutines and channels for concurrency..."},
    }
    
    // Create embeddings for documents
    ctx := context.Background()
    for i := range documents {
        embedding, err := getEmbedding(ctx, client, documents[i].Content)
        if err != nil {
            fmt.Printf("Error embedding document %d: %v\n", i, err)
            return
        }
        documents[i].Embedding = embedding
    }
    
    // Search loop
    scanner := bufio.NewScanner(os.Stdin)
    fmt.Println("Search (type 'exit' to quit):")
    
    for {
        fmt.Print("> ")
        if !scanner.Scan() {
            break
        }
        
        query := scanner.Text()
        if query == "exit" {
            break
        }
        
        // Search documents
        results, err := searchDocuments(ctx, client, query, documents)
        if err != nil {
            fmt.Printf("Error: %v\n", err)
            continue
        }
        
        // Display results
        fmt.Println("Results:")
        for i, doc := range results {
            if i >= 3 {
                break // Show top 3
            }
            fmt.Printf("%d. %s\n", i+1, doc.Title)
        }
        fmt.Println()
    }
}

func getEmbedding(ctx context.Context, client gai.Embedder[float64], text string) ([]float64, error) {
    response, err := client.Embed(ctx, gai.EmbedRequest{
        Input: strings.NewReader(text),
    })
    if err != nil {
        return nil, err
    }
    return response.Embedding, nil
}

func searchDocuments(ctx context.Context, client gai.Embedder[float64], query string, documents []Document) ([]Document, error) {
    // Embed query
    queryEmbedding, err := getEmbedding(ctx, client, query)
    if err != nil {
        return nil, err
    }
    
    // Score documents
    type scoredDoc struct {
        document Document
        score    float64
    }
    
    var scoredDocs []scoredDoc
    for _, doc := range documents {
        similarity := eval.CosineSimilarity(queryEmbedding, doc.Embedding)
        scoredDocs = append(scoredDocs, scoredDoc{
            document: doc,
            score:    float64(similarity),
        })
    }
    
    // Sort by score
    sort.Slice(scoredDocs, func(i, j int) bool {
        return scoredDocs[i].score > scoredDocs[j].score
    })
    
    // Convert back to documents
    result := make([]Document, len(scoredDocs))
    for i, sd := range scoredDocs {
        result[i] = sd.document
    }
    
    return result, nil
}
```

## Tool-Using Agent

Create an agent that uses tools to enhance its capabilities:

```go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "os"
    "time"

    "maragu.dev/gai"
    "maragu.dev/gai/tools"
    openai "github.com/maragudk/gai-openai"
)

func main() {
    // Initialize client
    client := openai.NewClient(os.Getenv("OPENAI_API_KEY"))
    
    // Create tools
    timeTool := tools.NewTime()
    
    // File tools with sandbox directory
    root := os.New("./sandbox")
    readFileTool := tools.NewReadFile(root)
    listDirTool := tools.NewListDir(root)
    editFileTool := tools.NewEditFile(root)
    
    // Create custom weather tool
    weatherTool := gai.Tool{
        Name:        "weather",
        Description: "Get the current weather for a location",
        Schema:      gai.GenerateSchema[struct {
            Location string `json:"location" jsonschema_description:"City name"`
        }](),
        Function: func(ctx context.Context, rawArgs json.RawMessage) (string, error) {
            var args struct {
                Location string `json:"location"`
            }
            if err := json.Unmarshal(rawArgs, &args); err != nil {
                return "", err
            }
            return fmt.Sprintf("It's currently 22°C and sunny in %s", args.Location), nil
        },
    }
    
    // Create initial prompt
    userPrompt := "What time is it, and what's the weather in Paris?"
    
    // Start conversation
    messages := []gai.Message{
        gai.NewUserTextMessage(userPrompt),
    }
    
    // Process conversation with tools
    ctx := context.Background()
    processConversation(ctx, client, messages, []gai.Tool{
        timeTool,
        weatherTool,
        readFileTool,
        listDirTool,
        editFileTool,
    })
}

func processConversation(ctx context.Context, client gai.ChatCompleter, messages []gai.Message, tools []gai.Tool) {
    request := gai.ChatCompleteRequest{
        Messages: messages,
        Tools:    tools,
    }
    
    response, err := client.ChatComplete(ctx, request)
    if err != nil {
        fmt.Printf("Error: %v\n", err)
        return
    }
    
    var fullText string
    var needToolExecution bool
    var toolCalls []gai.ToolCall
    
    // Process response
    for part, err := range response.Parts() {
        if err != nil {
            fmt.Printf("Error: %v\n", err)
            return
        }
        
        if part.Type == gai.MessagePartTypeText {
            text := part.Text()
            fullText += text
            fmt.Print(text)
        } else if part.Type == gai.MessagePartTypeToolCall {
            needToolExecution = true
            toolCall := part.ToolCall()
            toolCalls = append(toolCalls, toolCall)
            
            fmt.Printf("\n[Tool Call: %s]\n", toolCall.Name)
        }
    }
    
    if !needToolExecution {
        fmt.Println("\nDone.")
        return
    }
    
    // Execute tools and continue conversation
    var toolResults []gai.ToolResult
    for _, toolCall := range toolCalls {
        // Find the tool
        var tool *gai.Tool
        for i := range tools {
            if tools[i].Name == toolCall.Name {
                tool = &tools[i]
                break
            }
        }
        
        if tool == nil {
            fmt.Printf("Unknown tool: %s\n", toolCall.Name)
            continue
        }
        
        // Execute the tool
        fmt.Printf("Executing tool: %s\n", toolCall.Name)
        result, err := tool.Function(ctx, toolCall.Args)
        
        // Add result
        toolResults = append(toolResults, gai.ToolResult{
            ID:      toolCall.ID,
            Content: result,
            Err:     err,
        })
    }
    
    // Add model and tool results to conversation
    newMessages := append(messages, gai.Message{
        Role:  gai.MessageRoleModel,
        Parts: []gai.MessagePart{gai.TextMessagePart(fullText)},
    })
    
    // Add tool results
    for _, result := range toolResults {
        newMessages = append(newMessages, gai.NewUserToolResultMessage(result))
    }
    
    // Continue conversation
    processConversation(ctx, client, newMessages, tools)
}
```

## Evaluating Multiple Models

Compare different models on a specific task:

```go
package myapp_test

import (
    "context"
    "testing"

    "maragu.dev/gai"
    "maragu.dev/gai/eval"
    openai "github.com/maragudk/gai-openai"
    google "github.com/maragudk/gai-google"
    anthropic "github.com/maragudk/gai-anthropic"
)

func TestEvalModelComparison(t *testing.T) {
    // Test data
    questions := []string{
        "What is the capital of France?",
        "Explain quantum computing in simple terms",
        "Write a haiku about programming",
    }
    
    // Initialize models
    models := map[string]gai.ChatCompleter{
        "OpenAI-GPT4":     openai.NewClient(os.Getenv("OPENAI_API_KEY")),
        "Google-Gemini":   google.NewClient(os.Getenv("GOOGLE_API_KEY")),
        "Anthropic-Claude": anthropic.NewClient(os.Getenv("ANTHROPIC_API_KEY")),
    }
    
    // Create judge model for LLM evaluation
    judgeModel := openai.NewClient(os.Getenv("OPENAI_API_KEY"))
    
    // Run evaluations for each model
    for modelName, model := range models {
        eval.Run(t, modelName, func(t *testing.T, e *eval.E) {
            e.Group = "ModelComparison"
            
            for _, question := range questions {
                // Reset timer for fair comparison
                e.ResetTimer()
                
                // Get model response
                res, err := model.ChatComplete(t.Context(), gai.ChatCompleteRequest{
                    Messages: []gai.Message{
                        gai.NewUserTextMessage(question),
                    },
                })
                if err != nil {
                    t.Fatal(err)
                }
                
                // Collect response
                var output string
                for part, err := range res.Parts() {
                    if err != nil {
                        t.Fatal(err)
                    }
                    output += part.Text()
                }
                
                // Create sample (no golden answer for open-ended questions)
                sample := eval.Sample{
                    Input:    question,
                    Output:   output,
                    Expected: "", // No golden answer for open-ended evaluation
                }
                
                // Score with LLM judge
                judge := eval.RubricJudge(gai.Temperature(0.1))
                result := e.Score(sample, eval.LLMScorer(t, judge, judgeModel))
                
                // Log result
                e.Log(sample, result)
            }
        })
    }
}
```

## Memory Tool

Create a system with persistent memory:

```go
package main

import (
    "context"
    "fmt"
    "os"

    "maragu.dev/gai"
    "maragu.dev/gai/tools"
    openai "github.com/maragudk/gai-openai"
)

// Simple in-memory store
type memoryStore struct {
    memories []string
}

func (m *memoryStore) SaveMemory(ctx context.Context, memory string) error {
    m.memories = append(m.memories, memory)
    return nil
}

func (m *memoryStore) GetMemories(ctx context.Context) ([]string, error) {
    return m.memories, nil
}

func (m *memoryStore) SearchMemories(ctx context.Context, query string) ([]string, error) {
    // Simple string matching for demo
    var results []string
    for _, memory := range m.memories {
        if strings.Contains(strings.ToLower(memory), strings.ToLower(query)) {
            results = append(results, memory)
        }
    }
    return results, nil
}

func main() {
    client := openai.NewClient(os.Getenv("OPENAI_API_KEY"))
    
    // Create memory store
    store := &memoryStore{}
    
    // Create memory tools
    saveMemoryTool := tools.NewSaveMemory(store)
    getMemoriesTool := tools.NewGetMemories(store)
    searchMemoriesTool := tools.NewSearchMemories(store)
    
    // Handle user interaction and model processing
    system := "You're an assistant with memory. Use save_memory to remember important information. Use get_memories to recall all memories. Use search_memories to find specific memories."
    
    // Start chatbot loop
    // ... (Similar to previous chatbot example, but with memory tools)
}
```