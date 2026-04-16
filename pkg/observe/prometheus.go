package observe

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
)

// PrometheusHandler returns an http.Handler that serves metrics in Prometheus
// exposition format. Attach it to your HTTP server at "/metrics".
//
// Exposed metrics (per agent):
//
//	waggle_agent_runs_total{agent="name"} — total invocations
//	waggle_agent_success_total{agent="name"} — successful runs
//	waggle_agent_errors_total{agent="name"} — failed runs
//	waggle_agent_duration_seconds_total{agent="name"} — cumulative duration
//	waggle_agent_duration_seconds_min{agent="name"} — min duration
//	waggle_agent_duration_seconds_max{agent="name"} — max duration
//	waggle_agent_input_bytes_total{agent="name"} — total input bytes
//	waggle_agent_output_bytes_total{agent="name"} — total output bytes
//
// Example:
//
//	metrics := observe.NewMetrics()
//	http.Handle("/metrics", observe.PrometheusHandler(metrics))
func PrometheusHandler(m *Metrics) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

		all := m.All()
		// Sort for deterministic output.
		sort.Slice(all, func(i, j int) bool {
			return all[i].AgentName < all[j].AgentName
		})

		var sb strings.Builder

		sb.WriteString("# HELP waggle_agent_runs_total Total number of agent invocations.\n")
		sb.WriteString("# TYPE waggle_agent_runs_total counter\n")
		for _, ag := range all {
			fmt.Fprintf(&sb, "waggle_agent_runs_total{agent=%q} %d\n", ag.AgentName, ag.TotalRuns)
		}

		sb.WriteString("# HELP waggle_agent_success_total Total number of successful agent runs.\n")
		sb.WriteString("# TYPE waggle_agent_success_total counter\n")
		for _, ag := range all {
			fmt.Fprintf(&sb, "waggle_agent_success_total{agent=%q} %d\n", ag.AgentName, ag.SuccessRuns)
		}

		sb.WriteString("# HELP waggle_agent_errors_total Total number of failed agent runs.\n")
		sb.WriteString("# TYPE waggle_agent_errors_total counter\n")
		for _, ag := range all {
			fmt.Fprintf(&sb, "waggle_agent_errors_total{agent=%q} %d\n", ag.AgentName, ag.ErrorRuns)
		}

		sb.WriteString("# HELP waggle_agent_duration_seconds_total Cumulative agent execution time in seconds.\n")
		sb.WriteString("# TYPE waggle_agent_duration_seconds_total counter\n")
		for _, ag := range all {
			fmt.Fprintf(&sb, "waggle_agent_duration_seconds_total{agent=%q} %.6f\n", ag.AgentName, ag.TotalDuration.Seconds())
		}

		sb.WriteString("# HELP waggle_agent_duration_seconds_min Minimum agent execution time in seconds.\n")
		sb.WriteString("# TYPE waggle_agent_duration_seconds_min gauge\n")
		for _, ag := range all {
			fmt.Fprintf(&sb, "waggle_agent_duration_seconds_min{agent=%q} %.6f\n", ag.AgentName, ag.MinDuration.Seconds())
		}

		sb.WriteString("# HELP waggle_agent_duration_seconds_max Maximum agent execution time in seconds.\n")
		sb.WriteString("# TYPE waggle_agent_duration_seconds_max gauge\n")
		for _, ag := range all {
			fmt.Fprintf(&sb, "waggle_agent_duration_seconds_max{agent=%q} %.6f\n", ag.AgentName, ag.MaxDuration.Seconds())
		}

		sb.WriteString("# HELP waggle_agent_input_bytes_total Total input bytes processed.\n")
		sb.WriteString("# TYPE waggle_agent_input_bytes_total counter\n")
		for _, ag := range all {
			fmt.Fprintf(&sb, "waggle_agent_input_bytes_total{agent=%q} %d\n", ag.AgentName, ag.TotalInputBytes)
		}

		sb.WriteString("# HELP waggle_agent_output_bytes_total Total output bytes produced.\n")
		sb.WriteString("# TYPE waggle_agent_output_bytes_total counter\n")
		for _, ag := range all {
			fmt.Fprintf(&sb, "waggle_agent_output_bytes_total{agent=%q} %d\n", ag.AgentName, ag.TotalOutputBytes)
		}

		w.Write([]byte(sb.String()))
	})
}
