package observe

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPrometheusHandler_BasicOutput(t *testing.T) {
	m := NewMetrics()
	m.RecordStart("agent-a", 100)
	m.RecordSuccess("agent-a", 500*time.Millisecond, 200)
	m.RecordStart("agent-a", 150)
	m.RecordError("agent-a", 100*time.Millisecond)
	m.RecordStart("agent-b", 50)
	m.RecordSuccess("agent-b", 200*time.Millisecond, 80)

	handler := PrometheusHandler(m)
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	body := rec.Body.String()

	// Check content type.
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/plain") {
		t.Errorf("unexpected content type: %s", ct)
	}

	// Check key metrics are present.
	checks := []string{
		`waggle_agent_runs_total{agent="agent-a"} 2`,
		`waggle_agent_success_total{agent="agent-a"} 1`,
		`waggle_agent_errors_total{agent="agent-a"} 1`,
		`waggle_agent_runs_total{agent="agent-b"} 1`,
		`waggle_agent_input_bytes_total{agent="agent-a"} 250`,
		`waggle_agent_output_bytes_total{agent="agent-a"} 200`,
		`# TYPE waggle_agent_runs_total counter`,
		`# HELP waggle_agent_duration_seconds_total`,
	}
	for _, check := range checks {
		if !strings.Contains(body, check) {
			t.Errorf("missing in output: %q\n\nFull output:\n%s", check, body)
		}
	}
}

func TestPrometheusHandler_EmptyMetrics(t *testing.T) {
	m := NewMetrics()
	handler := PrometheusHandler(m)
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	body := rec.Body.String()
	// Should still have HELP/TYPE headers but no data lines with agent labels.
	if !strings.Contains(body, "# HELP") {
		t.Error("should have HELP headers even with no data")
	}
}

func TestPrometheusHandler_DeterministicOrder(t *testing.T) {
	m := NewMetrics()
	m.RecordStart("zebra", 10)
	m.RecordSuccess("zebra", time.Millisecond, 5)
	m.RecordStart("alpha", 10)
	m.RecordSuccess("alpha", time.Millisecond, 5)

	handler := PrometheusHandler(m)
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	body := rec.Body.String()
	alphaIdx := strings.Index(body, `agent="alpha"`)
	zebraIdx := strings.Index(body, `agent="zebra"`)
	if alphaIdx > zebraIdx {
		t.Error("agents should be sorted alphabetically")
	}
}
