package llm_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/generative-ai-go/genai"
	"maragu.dev/env"
	"maragu.dev/is"

	"maragu.dev/llm"
)

func TestNewGoogleClient(t *testing.T) {
	t.Run("can create a new client with a key", func(t *testing.T) {
		client := llm.NewGoogleClient(llm.NewGoogleClientOptions{Key: "123"})
		is.NotNil(t, client)
	})
}

func TestGoogleClientCompletion(t *testing.T) {
	_ = env.Load(".env.test.local")

	t.Run("can do a basic chat completion", func(t *testing.T) {
		client := llm.NewGoogleClient(llm.NewGoogleClientOptions{Key: env.GetStringOrDefault("GOOGLE_KEY", "")})
		is.NotNil(t, client)

		model := client.Client.GenerativeModel("models/gemini-1.5-flash-latest")
		model.SystemInstruction = genai.NewUserContent(genai.Text(`Only say the word "Hi", nothing more.`))
		res, err := model.GenerateContent(context.Background(), genai.Text("Hi."))
		is.NotError(t, err)
		is.True(t, len(res.Candidates) > 0)
		is.True(t, strings.Contains(fmt.Sprint(res.Candidates[0].Content.Parts[0]), "Hi"))
	})
}
