// Package oteltest provides test helpers for OpenTelemetry span assertions.
package oteltest

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// NewSpanRecorder installs a [tracetest.SpanRecorder] as the global [otel.TracerProvider]
// for the duration of the test and returns it. The previous provider is restored on cleanup.
// Construct clients under test AFTER calling NewSpanRecorder; tracers obtained via
// [otel.Tracer] earlier may continue routing to the previous provider and their spans
// will not reach the recorder.
// Not safe for parallel tests because it mutates global state.
// Inspired by github.com/maragudk/glue/oteltest.
func NewSpanRecorder(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()

	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))

	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)

	t.Cleanup(func() {
		_ = tp.Shutdown(context.WithoutCancel(t.Context()))
		otel.SetTracerProvider(previous)
	})

	return sr
}

// FindAttribute returns the value for the given key and true if the key is present,
// otherwise a zero value and false.
func FindAttribute(attrs []attribute.KeyValue, key attribute.Key) (attribute.Value, bool) {
	for _, attr := range attrs {
		if attr.Key == key {
			return attr.Value, true
		}
	}
	return attribute.Value{}, false
}

// HasAttribute reports whether the given attribute is present in the slice,
// matching both key and value.
func HasAttribute(attrs []attribute.KeyValue, want attribute.KeyValue) bool {
	v, ok := FindAttribute(attrs, want.Key)
	return ok && v == want.Value
}

// FindSpan returns the first recorded span matching name and fails the test if none exists.
func FindSpan(t *testing.T, spans []sdktrace.ReadOnlySpan, name string) sdktrace.ReadOnlySpan {
	t.Helper()
	for _, s := range spans {
		if s.Name() == name {
			return s
		}
	}
	t.Fatalf("span %q not found in %d recorded spans", name, len(spans))
	return nil
}

// SpansByName returns all recorded spans matching name, in recording order.
func SpansByName(spans []sdktrace.ReadOnlySpan, name string) []sdktrace.ReadOnlySpan {
	var out []sdktrace.ReadOnlySpan
	for _, s := range spans {
		if s.Name() == name {
			out = append(out, s)
		}
	}
	return out
}

// RequirePositiveIntAttribute fails the test if the attribute is missing or not > 0.
func RequirePositiveIntAttribute(t *testing.T, attrs []attribute.KeyValue, key string) {
	t.Helper()
	v, ok := FindAttribute(attrs, attribute.Key(key))
	if !ok {
		t.Fatalf("expected attribute %q to be present", key)
	}
	if v.AsInt64() <= 0 {
		t.Fatalf("expected attribute %q to be > 0, got %d", key, v.AsInt64())
	}
}

// RequireAttributePresent fails the test if the attribute is missing. It does not
// check the value, which is useful for attributes that may legitimately be zero
// (e.g. ai.cache_read_tokens on a cold call).
func RequireAttributePresent(t *testing.T, attrs []attribute.KeyValue, key string) {
	t.Helper()
	if _, ok := FindAttribute(attrs, attribute.Key(key)); !ok {
		t.Fatalf("expected attribute %q to be present", key)
	}
}

// RequireCacheReadSubsetOfPromptTokens fails the test if ai.cache_read_tokens is
// not less than or equal to ai.prompt_tokens. This invariant holds across all
// providers that emit both attributes (see PR #214).
func RequireCacheReadSubsetOfPromptTokens(t *testing.T, attrs []attribute.KeyValue) {
	t.Helper()
	promptV, ok := FindAttribute(attrs, attribute.Key("ai.prompt_tokens"))
	if !ok {
		t.Fatalf("expected attribute %q to be present", "ai.prompt_tokens")
	}
	cacheV, ok := FindAttribute(attrs, attribute.Key("ai.cache_read_tokens"))
	if !ok {
		t.Fatalf("expected attribute %q to be present", "ai.cache_read_tokens")
	}
	if cacheV.AsInt64() > promptV.AsInt64() {
		t.Fatalf("expected ai.cache_read_tokens (%d) <= ai.prompt_tokens (%d)", cacheV.AsInt64(), promptV.AsInt64())
	}
}
