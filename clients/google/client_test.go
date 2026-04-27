package google_test

import (
	"log/slog"
	"testing"

	"maragu.dev/env"
	"maragu.dev/is"

	"maragu.dev/gai/clients/google"
)

func TestNewClient(t *testing.T) {
	t.Run("can create a new client with a key", func(t *testing.T) {
		client := newClient(t)
		is.NotNil(t, client)
	})

	t.Run("can create a new client with the Vertex AI backend and an API key", func(t *testing.T) {
		client := newVertexAIClientWithKey(t)
		is.NotNil(t, client)
	})

	t.Run("can create a new client with the Vertex AI backend and a service account", func(t *testing.T) {
		client := newVertexAIClientWithCredentials(t)
		is.NotNil(t, client)
	})
}

func TestBackend(t *testing.T) {
	t.Run("has Gemini API and Vertex AI values", func(t *testing.T) {
		is.Equal(t, google.Backend("gemini"), google.BackendGemini)
		is.Equal(t, google.Backend("vertexai"), google.BackendVertexAI)
	})
}

func newVertexAIClientWithKey(t *testing.T) *google.Client {
	t.Helper()

	_ = env.Load("../../.env.test.local")

	log := slog.New(slog.NewTextHandler(&tWriter{t}, &slog.HandlerOptions{Level: slog.LevelDebug}))

	return google.NewClient(google.NewClientOptions{
		Backend: google.BackendVertexAI,
		Key:     env.GetStringOrDefault("GOOGLE_VERTEX_KEY", ""),
		Log:     log,
	})
}

func newVertexAIClientWithCredentials(t *testing.T) *google.Client {
	t.Helper()

	_ = env.Load("../../.env.test.local")

	log := slog.New(slog.NewTextHandler(&tWriter{t}, &slog.HandlerOptions{Level: slog.LevelDebug}))

	return google.NewClient(google.NewClientOptions{
		Backend:         google.BackendVertexAI,
		CredentialsPath: env.GetStringOrDefault("GOOGLE_VERTEX_CREDENTIALS_PATH", ""),
		Log:             log,
	})
}

func newClient(t *testing.T) *google.Client {
	t.Helper()

	_ = env.Load("../../.env.test.local")

	log := slog.New(slog.NewTextHandler(&tWriter{t}, &slog.HandlerOptions{Level: slog.LevelDebug}))

	return google.NewClient(google.NewClientOptions{
		Key: env.GetStringOrDefault("GOOGLE_KEY", ""),
		Log: log,
	})
}

type tWriter struct {
	t *testing.T
}

func (w *tWriter) Write(p []byte) (n int, err error) {
	w.t.Log(string(p))
	return len(p), nil
}
