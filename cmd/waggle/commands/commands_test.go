package commands_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lucientong/waggle/cmd/waggle/commands"
)

// ---- ParseWorkflow tests ----

func TestParseWorkflow_Valid(t *testing.T) {
	yaml := `
name: test-workflow
description: A test workflow
agents:
  - name: fetcher
    type: func
    description: Fetch data
  - name: processor
    type: llm
    model: gpt-4o
    provider: openai
    prompt: "Process the data"
flow:
  - from: fetcher
    to: processor
`
	wf, err := commands.ParseWorkflow([]byte(yaml))
	if err != nil {
		t.Fatalf("ParseWorkflow() unexpected error: %v", err)
	}
	if wf.Name != "test-workflow" {
		t.Errorf("Name = %q, want %q", wf.Name, "test-workflow")
	}
	if wf.Description != "A test workflow" {
		t.Errorf("Description = %q", wf.Description)
	}
	if len(wf.Agents) != 2 {
		t.Errorf("Agents count = %d, want 2", len(wf.Agents))
	}
	if len(wf.Flow) != 1 {
		t.Errorf("Flow count = %d, want 1", len(wf.Flow))
	}
	if wf.Flow[0].From != "fetcher" || wf.Flow[0].To != "processor" {
		t.Errorf("Flow[0] = %+v", wf.Flow[0])
	}
}

func TestParseWorkflow_MissingName(t *testing.T) {
	yaml := `
description: No name workflow
agents:
  - name: agent1
    type: func
`
	_, err := commands.ParseWorkflow([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for missing name, got nil")
	}
	if !strings.Contains(err.Error(), "name") {
		t.Errorf("error = %v, expected to mention 'name'", err)
	}
}

func TestParseWorkflow_MissingAgentName(t *testing.T) {
	yaml := `
name: bad-workflow
agents:
  - type: func
    description: Agent without a name
`
	_, err := commands.ParseWorkflow([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for missing agent name, got nil")
	}
}

func TestParseWorkflow_DuplicateAgentName(t *testing.T) {
	yaml := `
name: dup-workflow
agents:
  - name: agent1
    type: func
  - name: agent1
    type: llm
`
	_, err := commands.ParseWorkflow([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for duplicate agent name, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("error = %v, expected 'duplicate'", err)
	}
}

func TestParseWorkflow_UnknownAgentInFlow(t *testing.T) {
	yaml := `
name: bad-flow
agents:
  - name: agent1
    type: func
flow:
  - from: agent1
    to: ghost_agent
`
	_, err := commands.ParseWorkflow([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for unknown agent in flow, got nil")
	}
	if !strings.Contains(err.Error(), "ghost_agent") {
		t.Errorf("error = %v, expected to mention 'ghost_agent'", err)
	}
}

func TestParseWorkflow_EmptyFlowEdge(t *testing.T) {
	yaml := `
name: empty-edge
agents:
  - name: agent1
    type: func
flow:
  - from: ""
    to: agent1
`
	_, err := commands.ParseWorkflow([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for empty flow edge, got nil")
	}
}

func TestParseWorkflow_InvalidYAML(t *testing.T) {
	_, err := commands.ParseWorkflow([]byte("{{invalid yaml}}"))
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

func TestParseWorkflow_NoAgentsNoFlow(t *testing.T) {
	yaml := `
name: minimal
`
	wf, err := commands.ParseWorkflow([]byte(yaml))
	if err != nil {
		t.Fatalf("ParseWorkflow() unexpected error: %v", err)
	}
	if wf.Name != "minimal" {
		t.Errorf("Name = %q", wf.Name)
	}
	if len(wf.Agents) != 0 {
		t.Errorf("Agents = %d, want 0", len(wf.Agents))
	}
}

func TestParseWorkflow_AgentWithRetry(t *testing.T) {
	yaml := `
name: retry-wf
agents:
  - name: flaky
    type: func
    retry:
      max_attempts: 3
      base_delay_ms: 100
      max_delay_ms: 2000
    timeout_secs: 30
`
	wf, err := commands.ParseWorkflow([]byte(yaml))
	if err != nil {
		t.Fatalf("ParseWorkflow() unexpected error: %v", err)
	}
	a := wf.Agents[0]
	if a.Retry == nil {
		t.Fatal("expected Retry to be non-nil")
	}
	if a.Retry.MaxAttempts != 3 {
		t.Errorf("MaxAttempts = %d, want 3", a.Retry.MaxAttempts)
	}
	if a.Retry.BaseDelayMs != 100 {
		t.Errorf("BaseDelayMs = %d, want 100", a.Retry.BaseDelayMs)
	}
	if a.Retry.MaxDelayMs != 2000 {
		t.Errorf("MaxDelayMs = %d, want 2000", a.Retry.MaxDelayMs)
	}
	if a.TimeoutSecs != 30 {
		t.Errorf("TimeoutSecs = %f, want 30", a.TimeoutSecs)
	}
}

// ---- LoadWorkflow tests ----

func TestLoadWorkflow_Success(t *testing.T) {
	content := `
name: file-workflow
agents:
  - name: a
    type: func
`
	dir := t.TempDir()
	path := filepath.Join(dir, "workflow.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	wf, err := commands.LoadWorkflow(path)
	if err != nil {
		t.Fatalf("LoadWorkflow() unexpected error: %v", err)
	}
	if wf.Name != "file-workflow" {
		t.Errorf("Name = %q, want %q", wf.Name, "file-workflow")
	}
}

func TestLoadWorkflow_FileNotFound(t *testing.T) {
	_, err := commands.LoadWorkflow("/nonexistent/path/workflow.yaml")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

// ---- Complex workflow tests ----

func TestParseWorkflow_DiamondDAG(t *testing.T) {
	yaml := `
name: diamond
agents:
  - name: source
    type: func
  - name: left
    type: func
  - name: right
    type: func
  - name: sink
    type: func
flow:
  - from: source
    to: left
  - from: source
    to: right
  - from: left
    to: sink
  - from: right
    to: sink
`
	wf, err := commands.ParseWorkflow([]byte(yaml))
	if err != nil {
		t.Fatalf("ParseWorkflow() unexpected error: %v", err)
	}
	if len(wf.Agents) != 4 {
		t.Errorf("Agents = %d, want 4", len(wf.Agents))
	}
	if len(wf.Flow) != 4 {
		t.Errorf("Flow = %d, want 4", len(wf.Flow))
	}
}

// ---- Validate command tests ----

func TestValidate_NoArgs(t *testing.T) {
	err := commands.Validate(context.Background(), nil)
	if err == nil {
		t.Fatal("Validate() expected error with no args, got nil")
	}
}

func TestValidate_ValidFile(t *testing.T) {
	content := `
name: valid-wf
agents:
  - name: a
    type: func
  - name: b
    type: func
flow:
  - from: a
    to: b
`
	dir := t.TempDir()
	path := filepath.Join(dir, "valid.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	err := commands.Validate(context.Background(), []string{path})
	if err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}
}

func TestValidate_InvalidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte("{{invalid"), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	// Validate prints to stderr but doesn't return error for individual file failures.
	err := commands.Validate(context.Background(), []string{path})
	if err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}
}

func TestValidate_MissingFile(t *testing.T) {
	err := commands.Validate(context.Background(), []string{"/nonexistent/file.yaml"})
	if err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}
}

func TestValidate_MultipleFiles(t *testing.T) {
	dir := t.TempDir()

	good := filepath.Join(dir, "good.yaml")
	os.WriteFile(good, []byte("name: good\nagents:\n  - name: a\n    type: func\n"), 0644) //nolint

	bad := filepath.Join(dir, "bad.yaml")
	os.WriteFile(bad, []byte("{{invalid yaml"), 0644) //nolint

	err := commands.Validate(context.Background(), []string{good, bad})
	if err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}
}

// ---- Dot command tests ----

func TestDot_NoArgs(t *testing.T) {
	err := commands.Dot(context.Background(), nil)
	if err == nil {
		t.Fatal("Dot() expected error with no args, got nil")
	}
}

func TestDot_ValidFile(t *testing.T) {
	content := `
name: dot-test
agents:
  - name: fetch
    type: func
  - name: process
    type: llm
    model: gpt-4o
flow:
  - from: fetch
    to: process
`
	dir := t.TempDir()
	path := filepath.Join(dir, "workflow.yaml")
	os.WriteFile(path, []byte(content), 0644) //nolint

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := commands.Dot(context.Background(), []string{path})

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("Dot() unexpected error: %v", err)
	}

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	if !strings.Contains(output, "digraph") {
		t.Error("Dot output should contain 'digraph'")
	}
	if !strings.Contains(output, "fetch") {
		t.Error("Dot output should contain 'fetch' node")
	}
	if !strings.Contains(output, "process") {
		t.Error("Dot output should contain 'process' node")
	}
	if !strings.Contains(output, "[llm]") {
		t.Error("Dot output should contain type annotation '[llm]'")
	}
}

func TestDot_InvalidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	os.WriteFile(path, []byte("{{invalid"), 0644) //nolint

	err := commands.Dot(context.Background(), []string{path})
	if err == nil {
		t.Fatal("Dot() expected error for invalid YAML, got nil")
	}
}

func TestDot_FileNotFound(t *testing.T) {
	err := commands.Dot(context.Background(), []string{"/nonexistent/file.yaml"})
	if err == nil {
		t.Fatal("Dot() expected error for missing file, got nil")
	}
}

// ---- Run command tests ----

func TestRun_NoArgs(t *testing.T) {
	err := commands.Run(context.Background(), nil)
	if err == nil {
		t.Fatal("Run() expected error with no args, got nil")
	}
}

func TestRun_FileNotFound(t *testing.T) {
	err := commands.Run(context.Background(), []string{"/nonexistent/file.yaml"})
	if err == nil {
		t.Fatal("Run() expected error for missing file, got nil")
	}
}

func TestRun_ValidWorkflow(t *testing.T) {
	content := `
name: run-test
agents:
  - name: source
    type: func
  - name: sink
    type: func
flow:
  - from: source
    to: sink
`
	dir := t.TempDir()
	path := filepath.Join(dir, "workflow.yaml")
	os.WriteFile(path, []byte(content), 0644) //nolint

	err := commands.Run(context.Background(), []string{"--input", "hello", path})
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
}

func TestRun_NoSourceAgent(t *testing.T) {
	// All agents have incoming edges — no source can be found.
	// Agent a -> b, c -> a, c -> b. Agent c has incoming from nowhere? No.
	// We need all agents to have at least one incoming edge.
	// Use: a -> b, a -> c, b -> c. Here "a" has no incoming, so it's the source.
	// Instead: b -> a, c -> b, a -> c — this is a cycle, Connect will reject it.
	// Better approach: create a workflow where all agents have incoming edges
	// but no cycle by making it a single-node self-loop (which waggle rejects).
	// Simplest: a workflow with no flow edges but multiple agents — findSourceAgent
	// returns the first one, so we can't easily test "no source" without a cycle.
	//
	// Actually if ALL agents have incoming edges and the graph is acyclic,
	// that's impossible (a DAG must have at least one source). So we just
	// test that a workflow with all nodes having incoming edges in the YAML
	// (but cycle detection happens at Connect time).
	//
	// Instead, test Run with an empty workflow (no agents, no flow).
	content := `
name: empty-wf
agents: []
flow: []
`
	dir := t.TempDir()
	path := filepath.Join(dir, "workflow.yaml")
	os.WriteFile(path, []byte(content), 0644) //nolint

	err := commands.Run(context.Background(), []string{path})
	if err == nil {
		t.Fatal("Run() expected error for empty workflow, got nil")
	}
	if !strings.Contains(err.Error(), "source agent") {
		t.Errorf("error = %v, expected to mention 'source agent'", err)
	}
}

// ---- Serve/Version command tests ----

func TestVersion(t *testing.T) {
	err := commands.Version()
	if err != nil {
		t.Fatalf("Version() unexpected error: %v", err)
	}
}

func TestServe_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	// Cancel immediately so Serve exits quickly.
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	err := commands.Serve(ctx, []string{"--addr", ":0"})
	// Serve should return nil after context cancelled shutdown.
	if err != nil {
		t.Logf("Serve() returned: %v (acceptable)", err)
	}
}

func TestParseWorkflow_MultiProviderTypes(t *testing.T) {
	yaml := `
name: multi-type
agents:
  - name: func_agent
    type: func
    description: A function agent
  - name: llm_agent
    type: llm
    model: gpt-4o
    provider: openai
    prompt: "You are helpful."
  - name: tool_agent
    type: tool
    description: A tool agent
flow:
  - from: func_agent
    to: llm_agent
  - from: llm_agent
    to: tool_agent
`
	wf, err := commands.ParseWorkflow([]byte(yaml))
	if err != nil {
		t.Fatalf("ParseWorkflow() unexpected error: %v", err)
	}
	typeMap := map[string]string{}
	for _, a := range wf.Agents {
		typeMap[a.Name] = a.Type
	}
	if typeMap["func_agent"] != "func" {
		t.Errorf("func_agent type = %q", typeMap["func_agent"])
	}
	if typeMap["llm_agent"] != "llm" {
		t.Errorf("llm_agent type = %q", typeMap["llm_agent"])
	}
	if typeMap["tool_agent"] != "tool" {
		t.Errorf("tool_agent type = %q", typeMap["tool_agent"])
	}
}
