# Embedding

Embeddings turn text into vectors. They capture semantic meaning in a machine-understandable format.

## What Embeddings Do

Embeddings:
- Convert text to numerical vectors
- Position similar meanings close together 
- Enable similarity search
- Underpin RAG applications
- Allow clustering and classification

## Creating Embeddings

GAI makes embedding simple:

```go
request := gai.EmbedRequest{
    Input: strings.NewReader("The text you want to embed"),
}

response, err := client.Embed(context.Background(), request)
if err != nil {
    // Handle error
}

embedding := response.Embedding // Vector of float64 values
```

## Generic Vector Components

GAI supports different numeric types for vectors through generics:

```go
// Embedding with float32 instead of default float64
var client gai.Embedder[float32]
response, err := client.Embed(context.Background(), request)

// Embedding with int8 for quantized vectors
var quantizedClient gai.Embedder[int8]
quantizedResponse, err := quantizedClient.Embed(context.Background(), request)
```

This flexibility helps when:
- Saving memory with lower precision
- Working with quantized models
- Matching specific model output formats

## Vector Operations

Common operations with embeddings:

### Cosine Similarity

```go
// Using GAI's built-in function
similarity := eval.CosineSimilarity(embedding1.Embedding, embedding2.Embedding)
fmt.Printf("Cosine similarity: %f\n", similarity)
```

### Vector Storage

Store vectors in your preferred database:

```go
// Store in Postgres with pgvector
_, err = db.ExecContext(ctx, 
    "INSERT INTO documents (content, embedding) VALUES ($1, $2)",
    text, pgvector.NewVector(response.Embedding))
```

## Working with Multiple Texts

Embed multiple texts efficiently by batching requests yourself:

```go
texts := []string{
    "First text to embed",
    "Second text to embed",
    "Third text to embed",
}

var embeddings [][]float64
for _, text := range texts {
    request := gai.EmbedRequest{
        Input: strings.NewReader(text),
    }
    response, err := client.Embed(context.Background(), request)
    if err != nil {
        // Handle error
    }
    embeddings = append(embeddings, response.Embedding)
}
```

## Practical Application: Semantic Search

Basic semantic search implementation:

```go
func findSimilar(query string, documents []Document, client gai.Embedder[float64]) ([]Document, error) {
    // Embed the query
    queryEmbed, err := client.Embed(context.Background(), gai.EmbedRequest{
        Input: strings.NewReader(query),
    })
    if err != nil {
        return nil, err
    }
    
    // Calculate similarity to each document
    type docWithScore struct {
        doc   Document
        score float64
    }
    
    var scored []docWithScore
    for _, doc := range documents {
        similarity := eval.CosineSimilarity(queryEmbed.Embedding, doc.Embedding)
        scored = append(scored, docWithScore{doc: doc, score: float64(similarity)})
    }
    
    // Sort by similarity score
    sort.Slice(scored, func(i, j int) bool {
        return scored[i].score > scored[j].score
    })
    
    // Return documents sorted by relevance
    result := make([]Document, len(scored))
    for i, s := range scored {
        result[i] = s.doc
    }
    
    return result, nil
}
```

## Memory Efficiency

For large embeddings, consider:
- Using lower precision (float32 instead of float64)
- Quantizing vectors (int8)
- Dimensionality reduction techniques
- Implementing streaming for large datasets