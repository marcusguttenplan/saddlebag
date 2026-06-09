// Package trace implements W3C Trace Context (traceparent) generation and propagation.
// https://www.w3.org/TR/trace-context/
package trace

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
)

const (
	// TraceParentHeader is the W3C standard header name.
	TraceParentHeader = "traceparent"

	// TraceVersion is the W3C trace context version byte (always "00").
	TraceVersion = "00"
)

// Context holds the components of a parsed or generated traceparent.
type Context struct {
	// TraceID is a 16-byte (32 hex chars) unique identifier for the trace.
	TraceID string

	// SpanID is an 8-byte (16 hex chars) unique identifier for this span.
	SpanID string

	// Sampled indicates whether this trace is sampled (flags byte bit 0).
	Sampled bool
}

// New generates a fresh trace context with a new random trace ID and span ID.
func New() (*Context, error) {
	traceID, err := randomHex(16)
	if err != nil {
		return nil, fmt.Errorf("trace: generate trace ID: %w", err)
	}
	spanID, err := randomHex(8)
	if err != nil {
		return nil, fmt.Errorf("trace: generate span ID: %w", err)
	}
	return &Context{
		TraceID: traceID,
		SpanID:  spanID,
		Sampled: true,
	}, nil
}

// NewSpan derives a child span from an existing trace context.
// The trace ID is preserved; a new span ID is generated.
func (c *Context) NewSpan() (*Context, error) {
	spanID, err := randomHex(8)
	if err != nil {
		return nil, fmt.Errorf("trace: generate span ID: %w", err)
	}
	return &Context{
		TraceID: c.TraceID,
		SpanID:  spanID,
		Sampled: c.Sampled,
	}, nil
}

// Header returns the traceparent header value for this context.
// Format: "00-{traceID}-{spanID}-{flags}"
func (c *Context) Header() string {
	flags := "00"
	if c.Sampled {
		flags = "01"
	}
	return fmt.Sprintf("%s-%s-%s-%s", TraceVersion, c.TraceID, c.SpanID, flags)
}

// Parse parses a traceparent header value.
// Returns nil and no error if the header is empty (absent).
// Returns an error if the header is present but malformed.
func Parse(header string) (*Context, error) {
	if header == "" {
		return nil, nil
	}
	parts := strings.Split(header, "-")
	if len(parts) != 4 {
		return nil, fmt.Errorf("trace: invalid traceparent %q: expected 4 parts", header)
	}
	if parts[0] != TraceVersion {
		// Unknown version — best-effort parse as per spec.
		// We accept it but note it.
	}
	traceID := parts[1]
	spanID := parts[2]
	flags := parts[3]

	if len(traceID) != 32 {
		return nil, fmt.Errorf("trace: invalid trace ID length %d", len(traceID))
	}
	if len(spanID) != 16 {
		return nil, fmt.Errorf("trace: invalid span ID length %d", len(spanID))
	}

	sampled := flags == "01"
	return &Context{
		TraceID: traceID,
		SpanID:  spanID,
		Sampled: sampled,
	}, nil
}

// ParseOrNew parses the header if present; generates a fresh context if absent or invalid.
func ParseOrNew(header string) *Context {
	c, err := Parse(header)
	if err != nil || c == nil {
		c, _ = New()
	}
	return c
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
