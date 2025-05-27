# Tool Summarize Function Implementation Plan

## Overview
The Summarize field for `gai.Tool` should provide a concise summary of a tool call based on the supplied arguments. It should only summarize the arguments themselves - the tool name should not be duplicated in the summary.

## General Guidelines
- Keep summaries concise and readable
- Truncate long values appropriately (e.g., use ellipsis "..." for text over ~30 chars)
- Focus on the most important arguments
- Omit optional arguments if they use default values
- For tools with no arguments, return a simple descriptive phrase

## Tool-Specific Implementation Plans

### 1. exec.go - Command Execution Tool
**Summary Format:**
- With args: `command="<cmd>" args=[<arg1>,<arg2>,...]`
- Without args: `command="<cmd>"`
- With long args list: `command="<cmd>" args=[<arg1>,<arg2>,...] (X total)`
- With input: `command="<cmd>" input="<truncated...>"`
- With timeout: `command="<cmd>" timeout=<seconds>s`

**Implementation Notes:**
- Always show the command
- Show args if present (truncate list if >3 items)
- Show input if present (truncate to ~20 chars)
- Show timeout only if different from default (30s)

### 2. fetch.go - URL Fetching Tool
**Summary Format:**
- Basic: `url="<URL>"`
- With format: `url="<URL>" format="<format>"`

**Implementation Notes:**
- Always show the URL
- Only show output_format if explicitly specified
- URL should be shown in full unless extremely long (>100 chars)

### 3. file.go - File Operations Tools

#### read_file
**Summary Format:** `path="<filepath>"`

#### list_dir
**Summary Format:**
- With path: `path="<dirpath>"`
- Without path: `current directory`

#### edit_file
**Summary Format:** `path="<filepath>" search="<truncated...>" replace="<truncated...>"`

**Implementation Notes:**
- Truncate search_str and replace_str to ~20 chars each
- Show ellipsis if truncated

### 4. memory.go - Memory Management Tools

#### save_memory
**Summary Format:** `memory="<truncated...>"`

**Implementation Notes:**
- Truncate memory content to ~30 chars
- Show ellipsis if truncated

#### get_memories
**Summary Format:** `all memories`

#### search_memories
**Summary Format:** `query="<query>"`

### 5. time.go - Time Tool

#### get_time
**Summary Format:** `current time`

## Implementation Strategy

1. Add a Summarize field to each tool during creation
2. The Summarize function should:
   - Take the JSON RawMessage arguments as input
   - Unmarshal to the appropriate args struct
   - Build a summary string based on the format above
   - Return the summary string

## Example Implementation Pattern

```go
Summarize: func(rawArgs json.RawMessage) string {
    var args ArgsStruct
    if err := json.Unmarshal(rawArgs, &args); err != nil {
        return "error parsing arguments"
    }
    
    // Build summary based on args
    // ...
    
    return summary
}
```

## Utility Functions
Consider creating helper functions for:
- Truncating strings with ellipsis
- Formatting arrays/lists with counts
- Building key-value pair strings