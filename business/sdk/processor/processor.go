// Package processor is a client for the external (mock) payload processing
// service. Trace context is propagated automatically via the otelhttp transport.
package processor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// Request is the payload metadata sent to the processor.
type Request struct {
	PayloadID   string `json:"payload_id"`
	Kind        string `json:"kind"`
	Text        string `json:"text,omitempty"`
	ObjectKey   string `json:"object_key,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	SizeBytes   int64  `json:"size_bytes,omitempty"`
}

// Response is the result returned by the processor.
type Response struct {
	Result string `json:"result"`
}

// Client talks to the external processing service.
type Client struct {
	url    string
	client *http.Client
}

// New constructs a processor client. The otelhttp transport creates a client
// span per call and injects the W3C trace context into the request headers.
func New(url string, timeout time.Duration) *Client {
	return &Client{
		url: url,
		client: &http.Client{
			Timeout:   timeout,
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		},
	}
}

// Process sends a payload to the external processor and returns its result.
func (c *Client) Process(ctx context.Context, req Request) (Response, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return Response{}, fmt.Errorf("marshal: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return Response{}, fmt.Errorf("new request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return Response{}, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode != http.StatusOK {
		return Response{}, fmt.Errorf("processor returned status %d: %s", resp.StatusCode, string(data))
	}

	var out Response
	if err := json.Unmarshal(data, &out); err != nil {
		return Response{}, fmt.Errorf("unmarshal: %w", err)
	}

	return out, nil
}
