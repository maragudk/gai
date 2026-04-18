package oteltest_test

import (
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"maragu.dev/is"

	"maragu.dev/gai/internal/oteltest"
)

func TestNewSpanRecorder(t *testing.T) {
	t.Run("records spans emitted through the global tracer provider", func(t *testing.T) {
		sr := oteltest.NewSpanRecorder(t)

		_, span := otel.Tracer("test").Start(t.Context(), "glitter-span")
		span.End()

		spans := sr.Ended()
		is.Equal(t, 1, len(spans))
		is.Equal(t, "glitter-span", spans[0].Name())
	})

	t.Run("restores the previous tracer provider after cleanup", func(t *testing.T) {
		previous := otel.GetTracerProvider()

		t.Run("inner", func(t *testing.T) {
			oteltest.NewSpanRecorder(t)
		})

		is.Equal(t, previous, otel.GetTracerProvider())
	})
}

func TestFindAttribute(t *testing.T) {
	attrs := []attribute.KeyValue{
		attribute.String("color", "purple"),
		attribute.Int("sparkle_count", 42),
	}

	t.Run("returns value and true when key is present", func(t *testing.T) {
		v, ok := oteltest.FindAttribute(attrs, attribute.Key("color"))
		is.True(t, ok)
		is.Equal(t, "purple", v.AsString())
	})

	t.Run("returns zero value and false when key is missing", func(t *testing.T) {
		_, ok := oteltest.FindAttribute(attrs, attribute.Key("missing"))
		is.True(t, !ok)
	})
}

func TestHasAttribute(t *testing.T) {
	attrs := []attribute.KeyValue{
		attribute.String("color", "purple"),
		attribute.Int("sparkle_count", 42),
	}

	t.Run("matches present key and value", func(t *testing.T) {
		is.True(t, oteltest.HasAttribute(attrs, attribute.String("color", "purple")))
		is.True(t, oteltest.HasAttribute(attrs, attribute.Int("sparkle_count", 42)))
	})

	t.Run("does not match when value differs", func(t *testing.T) {
		is.True(t, !oteltest.HasAttribute(attrs, attribute.String("color", "teal")))
	})

	t.Run("does not match when key is missing", func(t *testing.T) {
		is.True(t, !oteltest.HasAttribute(attrs, attribute.String("missing", "purple")))
	})
}
