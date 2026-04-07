package web_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lucientong/waggle/pkg/agent"
	"github.com/lucientong/waggle/pkg/waggle"
	"github.com/lucientong/waggle/pkg/web"
)

func TestAPIDAG_WithWaggle(t *testing.T) {
	w := waggle.New()
	a := agent.Erase(agent.Func[any, any]("fetch", func(_ context.Context, in any) (any, error) {
		return in, nil
	}))
	b := agent.Erase(agent.Func[any, any]("process", func(_ context.Context, in any) (any, error) {
		return in, nil
	}))

	if err := w.Register(a); err != nil {
		t.Fatalf("Register(a): %v", err)
	}
	if err := w.Register(b); err != nil {
		t.Fatalf("Register(b): %v", err)
	}
	if err := w.Connect("fetch", "process"); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	srv := web.NewServer(web.DefaultConfig(), w, nil)
	defer srv.Close()
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := ts.Client().Get(ts.URL + "/api/dag")
	if err != nil {
		t.Fatalf("GET /api/dag error: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	var body struct {
		Nodes []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"nodes"`
		Edges []struct {
			From string `json:"from"`
			To   string `json:"to"`
		} `json:"edges"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if len(body.Nodes) != 2 {
		t.Errorf("nodes count = %d, want 2", len(body.Nodes))
	}
	if len(body.Edges) < 1 {
		t.Errorf("edges count = %d, want >= 1", len(body.Edges))
	}

	// Verify node names.
	names := make(map[string]bool)
	for _, n := range body.Nodes {
		names[n.Name] = true
	}
	if !names["fetch"] {
		t.Error("missing node 'fetch'")
	}
	if !names["process"] {
		t.Error("missing node 'process'")
	}
}

func TestStaticFiles_JS(t *testing.T) {
	srv := web.NewServer(web.DefaultConfig(), nil, nil)
	defer srv.Close()
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Request a known static asset path.
	resp, err := ts.Client().Get(ts.URL + "/app.js")
	if err != nil {
		t.Fatalf("GET /app.js error: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Static file should be served (200) or not found (404) but not error.
	if resp.StatusCode >= 500 {
		t.Errorf("status = %d, expected non-5xx", resp.StatusCode)
	}
}
