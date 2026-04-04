package web_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lucientong/waggle/pkg/observe"
	"github.com/lucientong/waggle/pkg/web"
)

// newTestServer creates a Server with test defaults (nil waggle/metrics).
func newTestServer(t *testing.T) *web.Server {
	t.Helper()
	return web.NewServer(web.DefaultConfig(), nil, nil)
}

func TestHealth(t *testing.T) {
	srv := newTestServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := ts.Client().Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

func TestAPIDAG_Empty(t *testing.T) {
	srv := newTestServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := ts.Client().Get(ts.URL + "/api/dag")
	if err != nil {
		t.Fatalf("GET /api/dag error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	resp.Body.Close()

	if _, ok := body["nodes"]; !ok {
		t.Error("response missing 'nodes' field")
	}
	if _, ok := body["edges"]; !ok {
		t.Error("response missing 'edges' field")
	}
}

func TestAPIMetrics_Empty(t *testing.T) {
	srv := newTestServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := ts.Client().Get(ts.URL + "/api/metrics")
	if err != nil {
		t.Fatalf("GET /api/metrics error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body) //nolint
	if _, ok := body["agents"]; !ok {
		t.Error("response missing 'agents' field")
	}
}

func TestAPIMetrics_WithData(t *testing.T) {
	m := observe.NewMetrics()
	m.RecordStart("fetcher", 100)
	m.RecordSuccess("fetcher", 0, 200)

	srv := web.NewServer(web.DefaultConfig(), nil, m)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := ts.Client().Get(ts.URL + "/api/metrics")
	if err != nil {
		t.Fatalf("GET /api/metrics error: %v", err)
	}
	defer resp.Body.Close()

	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body) //nolint

	agents, ok := body["agents"].([]any)
	if !ok || len(agents) == 0 {
		t.Fatal("expected at least one agent metric")
	}

	agentMap := agents[0].(map[string]any)
	if agentMap["agent_name"] != "fetcher" {
		t.Errorf("agent_name = %v, want 'fetcher'", agentMap["agent_name"])
	}
}

func TestContentTypeJSON(t *testing.T) {
	srv := newTestServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	for _, path := range []string{"/api/dag", "/api/metrics"} {
		resp, err := ts.Client().Get(ts.URL + path)
		if err != nil {
			t.Fatalf("GET %s error: %v", path, err)
		}
		resp.Body.Close()
		ct := resp.Header.Get("Content-Type")
		if !strings.HasPrefix(ct, "application/json") {
			t.Errorf("GET %s Content-Type = %q, want application/json", path, ct)
		}
	}
}
