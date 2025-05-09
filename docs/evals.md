# Evaluations

GAI provides tools to evaluate model outputs. Test how well models perform on your specific tasks.

## Why Evaluate Models

Evaluations help you:
- Compare model performance
- Track quality over time
- Identify areas for improvement
- Verify behavior on edge cases
- Make informed provider choices

## Core Evaluation Concepts

GAI's evaluation system uses:

- **Samples**: Input, output, and expected output triplets
- **Scorers**: Functions that evaluate outputs against expectations
- **Results**: Score and metadata from evaluation
- **Logs**: Structured records of evaluations

## Running Evaluations

GAI runs evaluations as part of normal Go tests:

```go
package myapp_test

import (
    "context"
    "testing"

    "maragu.dev/gai"
    "maragu.dev/gai/eval"
)

// TestEvalAccuracy evaluates model output accuracy.
// Must be prefixed with "TestEval" to be detected.
func TestEvalAccuracy(t *testing.T) {
    eval.Run(t, "capital cities accuracy", func(t *testing.T, e *eval.E) {
        // Initialize model client
        model := initializeModel()

        // Send request to model
        input := "What is the capital of France?"
        res, err := model.ChatComplete(t.Context(), gai.ChatCompleteRequest{
            Messages: []gai.Message{
                gai.NewUserTextMessage(input),
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

        // Create evaluation sample
        sample := eval.Sample{
            Input:    input,
            Output:   output,
            Expected: "Paris is the capital of France.",
        }

        // Score using lexical similarity
        lexicalResult := e.Score(sample, eval.LexicalSimilarityScorer(eval.LevenshteinDistance))

        // Score using semantic similarity
        semanticResult := e.Score(sample, eval.SemanticSimilarityScorer(t, model, eval.CosineSimilarity))

        // Log results
        e.Log(sample, lexicalResult, semanticResult)
    })
}
```

## Running Eval Tests

Evaluations only run when you explicitly include them:

```sh
# Run all evaluations
go test -run TestEval ./...

# Run specific evaluations
go test -run TestEvalAccuracy ./...
```

## Scoring Methods

GAI includes multiple scoring methods:

### Lexical Similarity

Compare text strings directly:

```go
// Using Levenshtein distance
lexicalResult := e.Score(sample, eval.LexicalSimilarityScorer(eval.LevenshteinDistance))

// Exact match (1.0 if identical, 0.0 otherwise)
exactResult := e.Score(sample, eval.LexicalSimilarityScorer(eval.ExactMatch))

// Contains match (1.0 if output contains expected, 0.0 otherwise)
containsResult := e.Score(sample, eval.LexicalSimilarityScorer(eval.Contains))
```

### Semantic Similarity

Compare meaning using embeddings:

```go
// Using cosine similarity on embeddings
semanticResult := e.Score(sample, eval.SemanticSimilarityScorer(t, model, eval.CosineSimilarity))
```

### LLM Judge

Use another LLM to judge outputs:

```go
// Basic judge using default prompt and temperature
judge := eval.DefaultJudge(gai.Temperature(0.1))
llmResult := e.Score(sample, eval.LLMScorer(t, judge, judgeModel))

// Rubric-based judge for multi-dimensional evaluation
rubricJudge := eval.RubricJudge(gai.Temperature(0.1))
rubricResult := e.Score(sample, eval.LLMScorer(t, rubricJudge, judgeModel))
```

## Evaluation Groups

Group evaluations for comparison:

```go
e.Group = "FactualAccuracy"
```

## Timing Information

Reset timer to focus on specific operations:

```go
// Reset timer before evaluated operation
e.ResetTimer()

// Perform operation to time
output := model.GenerateResponse(input)

// Log with timing information
e.Log(sample, result)
```

## Real-World Example

Comprehensive evaluation suite:

```go
func TestEvalSummarization(t *testing.T) {
    eval.Run(t, "article summarization", func(t *testing.T, e *eval.E) {
        // Load test articles
        articles := loadTestArticles()
        
        // Test each article
        for _, article := range articles {
            // Reset timer for fair performance comparison
            e.ResetTimer()
            
            // Get model summary
            res, err := model.ChatComplete(t.Context(), gai.ChatCompleteRequest{
                Messages: []gai.Message{
                    gai.NewUserTextMessage("Summarize this article: " + article.Content),
                },
            })
            if err != nil {
                t.Fatal(err)
            }
            
            // Collect response
            var summary string
            for part, err := range res.Parts() {
                if err != nil {
                    t.Fatal(err)
                }
                summary += part.Text()
            }
            
            // Create sample with article's human-written summary as expected
            sample := eval.Sample{
                Input:    article.Content,
                Output:   summary,
                Expected: article.HumanSummary,
            }
            
            // Score with multiple methods
            semantic := e.Score(sample, eval.SemanticSimilarityScorer(t, model, eval.CosineSimilarity))
            llm := e.Score(sample, eval.LLMScorer(t, eval.RubricJudge(0.1), judgeModel))
            
            // Log results
            e.Log(sample, semantic, llm)
        }
    })
}
```

## Visualization

GAI writes evaluation results to `evals.jsonl` in your project root.

You can visualize results with:
- Custom dashboards
- JSONL processors
- Visualization libraries

Example web dashboard integration:

```go
// Configure data export to dashboard service
os.Setenv("EVALS_API_KEY", "your-api-key")
os.Setenv("EVALS_PROJECT", "your-project")

// Run evals as normal - will automatically upload results
go test -run TestEval ./...
```

Results appear at https://api.evals.fun/evals.svg?key=your-api-key.