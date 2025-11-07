package anthropic_test

import (
	"log/slog"
	"testing"

	"maragu.dev/env"
	"maragu.dev/is"

	"maragu.dev/gai/clients/anthropic"
)

func TestNewClient(t *testing.T) {
	t.Run("can create a new client with a key", func(t *testing.T) {
		client := newClient(t)
		is.NotNil(t, client)
	})
}

func newClient(t *testing.T) *anthropic.Client {
	t.Helper()

	_ = env.Load("../../.env.test.local")

	log := slog.New(slog.NewTextHandler(&tWriter{t}, &slog.HandlerOptions{Level: slog.LevelDebug}))

	return anthropic.NewClient(anthropic.NewClientOptions{
		Key: env.GetStringOrDefault("ANTHROPIC_KEY", ""),
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
