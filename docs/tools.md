# Tools

Tools extend model capabilities. They let models interact with external systems to fetch information or perform actions.

## Core Concepts

A tool in GAI consists of:
- A name
- A description
- A schema for arguments
- A function to execute

## Defining Tools

Define a tool with these components:

```go
tool := gai.Tool{
    Name:        "weather",
    Description: "Get current weather for a location",
    Schema:      gai.GenerateSchema[WeatherArgs](),
    Function:    weatherFunction,
}
```

### Argument Schema

Define argument types with structs and JSON tags:

```go
type WeatherArgs struct {
    Location string `json:"location" jsonschema_description:"City name or coordinates"`
    Units    string `json:"units,omitempty" jsonschema_description:"Units (metric/imperial)"`
}
```

The `jsonschema_description` tags provide guidance to models.

### Tool Function

Implement the execution logic:

```go
func weatherFunction(ctx context.Context, rawArgs json.RawMessage) (string, error) {
    var args WeatherArgs
    if err := json.Unmarshal(rawArgs, &args); err != nil {
        return "", fmt.Errorf("error parsing weather args: %w", err)
    }
    
    // Call weather API, etc.
    weather := fetchWeather(args.Location, args.Units)
    
    return fmt.Sprintf("Current temperature in %s: %.1f°C", args.Location, weather.Temperature), nil
}
```

## Using Tools with Chat Completion

Add tools to chat requests:

```go
request := gai.ChatCompleteRequest{
    Messages: []gai.Message{
        gai.NewUserTextMessage("What's the weather in New York?"),
    },
    Tools: []gai.Tool{weatherTool, timeTool},
}

response, err := client.ChatComplete(context.Background(), request)
if err != nil {
    // Handle error
}
```

## Handling Tool Calls

Process tool calls in response stream:

```go
for part, err := range response.Parts() {
    if err != nil {
        // Handle error
        break
    }
    
    if part.Type == gai.MessagePartTypeToolCall {
        toolCall := part.ToolCall()
        
        // Execute the tool
        var result string
        var toolErr error
        
        if toolCall.Name == "weather" {
            result, toolErr = handleWeatherTool(ctx, toolCall.Args)
        }
        
        // Send tool result back to model
        toolResult := gai.ToolResult{
            ID:      toolCall.ID,
            Content: result,
            Err:     toolErr,
        }
        
        // Add tool result to conversation
        toolResultMsg := gai.NewUserToolResultMessage(toolResult)
        
        // Continue conversation with tool result
        continueRequest := gai.ChatCompleteRequest{
            Messages: append(request.Messages, toolResultMsg),
            Tools:    request.Tools,
        }
        
        // Get new response...
    } else if part.Type == gai.MessagePartTypeText {
        // Handle text response
        fmt.Print(part.Text())
    }
}
```

## Included Tools

GAI includes ready-to-use tools:

### File Tools

File access tools for reading and writing:

```go
root := os.New("/path/to/files")
readFileTool := tools.NewReadFile(root)
listDirTool := tools.NewListDir(root)
editFileTool := tools.NewEditFile(root)
```

### Memory Tools

Tools for storing and retrieving information:

```go
// Create memory storage backend
memoryStore := memoryStore{}

// Create memory tools
saveMemoryTool := tools.NewSaveMemory(memoryStore)
getMemoriesTool := tools.NewGetMemories(memoryStore)
searchMemoriesTool := tools.NewSearchMemories(memoryStore)
```

### Time Tool

Get current time information:

```go
timeTool := tools.NewTime()
```

## Complete Example

A complete example using weather and time tools:

```go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "log"
    "os"
    "time"

    "maragu.dev/gai"
    "maragu.dev/gai/tools"
    openai "github.com/maragudk/gai-openai"
)

type WeatherArgs struct {
    Location string `json:"location" jsonschema_description:"City name or coordinates"`
}

func main() {
    // Create tools
    weatherTool := gai.Tool{
        Name:        "weather",
        Description: "Get current weather for a location",
        Schema:      gai.GenerateSchema[WeatherArgs](),
        Function: func(ctx context.Context, rawArgs json.RawMessage) (string, error) {
            var args WeatherArgs
            if err := json.Unmarshal(rawArgs, &args); err != nil {
                return "", err
            }
            return fmt.Sprintf("It's sunny and 22°C in %s", args.Location), nil
        },
    }
    
    timeTool := tools.NewTime()
    
    // Create client
    client := openai.NewClient(os.Getenv("OPENAI_API_KEY"))
    
    // Create request with tools
    request := gai.ChatCompleteRequest{
        Messages: []gai.Message{
            gai.NewUserTextMessage("What's the weather in Paris and what time is it?"),
        },
        Tools: []gai.Tool{weatherTool, timeTool},
    }
    
    // Use the tools in a conversation
    conductConversation(client, request)
}

// For brevity, implementation of conductConversation is omitted
```

## Best Practices

- Keep tool descriptions clear and concise
- Handle errors gracefully
- Provide informative error messages
- Be specific about argument requirements
- Limit tool execution time
- Return structured data when possible