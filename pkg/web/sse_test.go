package web_test

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lucientong/waggle/pkg/observe"
	"github.com/lucientong/waggle/pkg/web"
)

func TestSSE_ConnectedPing(t *testing.T) {
	srv := web.NewServer(web.DefaultConfig(), nil, nil)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Start the server's internal SSE hub (normally done by srv.Start()).
	go func() {
		// Access the SSE endpoint to trigger hub registration.
		// The hub's run loop is started via the SSE handler's first connection.
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/events", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("GET /api/events error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}

	// Read the first line — should be the connected ping.
	scanner := bufio.NewScanner(resp.Body)
	if scanner.Scan() {
		line := scanner.Text()
		if !strings.Contains(line, "connected") {
			t.Errorf("first SSE message = %q, expected 'connected' ping", line)
		}
	} else {
		t.Error("no data received from SSE endpoint")
	}
}

func TestSSE_PublishEvent(t *testing.T) {
	srv := web.NewServer(web.DefaultConfig(), nil, nil)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/events", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("GET /api/events error: %v", err)
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)

	// Skip the "connected" ping message.
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "connected") {
			break
		}
	}

	// Publish an event after a short delay to ensure the client is registered.
	go func() {
		time.Sleep(100 * time.Millisecond)
		srv.PublishEvent(observe.NewAgentStartEvent("wf-sse", "test-agent", 100))
	}()

	// Read the next data line (the published event).
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue // skip empty lines between SSE messages
		}
		if strings.Contains(line, "test-agent") {
			return // success
		}
	}

	t.Error("did not receive the published event via SSE")
}

func TestSSE_EventChannel(t *testing.T) {
	srv := web.NewServer(web.DefaultConfig(), nil, nil)

	// EventChannel should return a writable channel.
	ch := srv.EventChannel()
	if ch == nil {
		t.Fatal("EventChannel() returned nil")
	}

	// Should be able to send without blocking (buffer size is 256).
	select {
	case ch <- observe.NewAgentStartEvent("wf-ch", "agent-ch", 50):
		// ok
	default:
		t.Error("EventChannel should accept events without blocking")
	}
}

func TestSSE_Headers(t *testing.T) {
	srv := web.NewServer(web.DefaultConfig(), nil, nil)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/events", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("GET /api/events error: %v", err)
	}
	defer resp.Body.Close()

	expectedHeaders := map[string]string{
		"Content-Type":                "text/event-stream",
		"Cache-Control":              "no-cache",
		"Access-Control-Allow-Origin": "*",
	}
	for key, want := range expectedHeaders {
		got := resp.Header.Get(key)
		if got != want {
			t.Errorf("Header %q = %q, want %q", key, got, want)
		}
	}
}
