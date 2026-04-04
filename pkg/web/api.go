package web

import (
	"encoding/json"
	"net/http"
	"time"
)

// dagResponse is the JSON response for GET /api/dag.
type dagResponse struct {
	Nodes []dagNodeJSON `json:"nodes"`
	Edges []dagEdgeJSON `json:"edges"`
}

type dagNodeJSON struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Status       string   `json:"status"`
	Predecessors []string `json:"predecessors"`
	Successors   []string `json:"successors"`
}

type dagEdgeJSON struct {
	From      string `json:"from"`
	To        string `json:"to"`
	LatencyMs int64  `json:"latency_ms,omitempty"`
	DataSize  int    `json:"data_size,omitempty"`
}

// metricsResponse is the JSON response for GET /api/metrics.
type metricsResponse struct {
	Agents []agentMetricsJSON `json:"agents"`
}

type agentMetricsJSON struct {
	AgentName        string  `json:"agent_name"`
	TotalRuns        int64   `json:"total_runs"`
	SuccessRuns      int64   `json:"success_runs"`
	ErrorRuns        int64   `json:"error_runs"`
	ErrorRate        float64 `json:"error_rate"`
	AvgDurationMs    int64   `json:"avg_duration_ms"`
	MinDurationMs    int64   `json:"min_duration_ms"`
	MaxDurationMs    int64   `json:"max_duration_ms"`
	TotalInputBytes  int64   `json:"total_input_bytes"`
	TotalOutputBytes int64   `json:"total_output_bytes"`
}

// handleDAG serves the current DAG structure.
func (s *Server) handleDAG(w http.ResponseWriter, r *http.Request) {
	resp := dagResponse{
		Nodes: []dagNodeJSON{},
		Edges: []dagEdgeJSON{},
	}

	if s.waggleRef != nil {
		info := s.waggleRef.DAGInfo()
		for _, n := range info.Nodes {
			resp.Nodes = append(resp.Nodes, dagNodeJSON{
				ID:           n.ID,
				Name:         n.Name,
				Status:       "waiting",
				Predecessors: n.Predecessors,
				Successors:   n.Successors,
			})
			// Build edges from successors list.
			for _, succ := range n.Successors {
				resp.Edges = append(resp.Edges, dagEdgeJSON{From: n.ID, To: succ})
			}
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleMetrics serves aggregated agent metrics.
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	resp := metricsResponse{
		Agents: []agentMetricsJSON{},
	}

	if s.metrics != nil {
		for _, m := range s.metrics.All() {
			resp.Agents = append(resp.Agents, agentMetricsJSON{
				AgentName:        m.AgentName,
				TotalRuns:        m.TotalRuns,
				SuccessRuns:      m.SuccessRuns,
				ErrorRuns:        m.ErrorRuns,
				ErrorRate:        m.ErrorRate(),
				AvgDurationMs:    m.AvgDuration().Milliseconds(),
				MinDurationMs:    m.MinDuration.Milliseconds(),
				MaxDurationMs:    m.MaxDuration.Milliseconds(),
				TotalInputBytes:  m.TotalInputBytes,
				TotalOutputBytes: m.TotalOutputBytes,
			})
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// writeJSON encodes v as JSON and writes it to w with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, "json encode error", http.StatusInternalServerError)
	}
}

// eventJSON is the JSON representation of an event sent over SSE.
type eventJSON struct {
	Type       string         `json:"type"`
	AgentName  string         `json:"agent_name,omitempty"`
	WorkflowID string         `json:"workflow_id,omitempty"`
	Timestamp  time.Time      `json:"timestamp"`
	DurationMs int64          `json:"duration_ms,omitempty"`
	Error      string         `json:"error,omitempty"`
	InputSize  int            `json:"input_size,omitempty"`
	OutputSize int            `json:"output_size,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}
