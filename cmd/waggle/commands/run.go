package commands

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/lucientong/waggle/pkg/agent"
	"github.com/lucientong/waggle/pkg/waggle"
)

// Run executes the `waggle run <workflow.yaml>` command.
func Run(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	input := fs.String("input", "", "Input value to pass to the first agent")
	timeout := fs.Duration("timeout", 5*time.Minute, "Maximum total execution time")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return fmt.Errorf("run: workflow YAML file path required\nUsage: waggle run [flags] <workflow.yaml>")
	}

	wfPath := fs.Arg(0)
	wf, err := LoadWorkflow(wfPath)
	if err != nil {
		return fmt.Errorf("run: %w", err)
	}

	slog.Info("running workflow", "name", wf.Name, "agents", len(wf.Agents), "edges", len(wf.Flow))

	// Build the Waggle orchestrator from the workflow definition.
	w := waggle.New()
	if err := buildOrchestratorFromWorkflow(w, wf); err != nil {
		return fmt.Errorf("run: build orchestrator: %w", err)
	}

	// Find the source node (no predecessors in the flow).
	startID := findSourceAgent(wf)
	if startID == "" {
		return fmt.Errorf("run: could not determine source agent; ensure the workflow has a node with no incoming edges")
	}

	// Apply total execution timeout.
	execCtx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()

	inputVal := any(*input)
	start := time.Now()
	result, err := w.RunFrom(execCtx, startID, inputVal)
	elapsed := time.Since(start)

	if err != nil {
		slog.Error("workflow failed", "duration", elapsed, "error", err)
		return fmt.Errorf("run: workflow failed after %s: %w", elapsed.Round(time.Millisecond), err)
	}

	slog.Info("workflow completed", "duration", elapsed)
	fmt.Printf("Result: %v\n", result)
	return nil
}

// buildOrchestratorFromWorkflow registers agents and edges into a Waggle orchestrator.
// For YAML-defined workflows, agents are created as echo/passthrough agents since
// actual function/LLM implementations must be provided by user code.
func buildOrchestratorFromWorkflow(w *waggle.Waggle, wf *WorkflowDefinition) error {
	// Register all agents as type-erased passthrough agents for demonstration.
	// In production, users wire their own agent implementations before calling Run.
	for _, a := range wf.Agents {
		agentDef := a
		echoAgent := agent.Erase(agent.Func[any, any](agentDef.Name, func(_ context.Context, input any) (any, error) {
			// Passthrough: in a real scenario this would be replaced by the actual implementation.
			return fmt.Sprintf("[%s] processed: %v", agentDef.Name, input), nil
		}))
		if err := w.Register(echoAgent); err != nil {
			return fmt.Errorf("register agent %q: %w", agentDef.Name, err)
		}
	}

	// Wire edges.
	for _, edge := range wf.Flow {
		if err := w.Connect(edge.From, edge.To); err != nil {
			return fmt.Errorf("connect %q -> %q: %w", edge.From, edge.To, err)
		}
	}

	return nil
}

// findSourceAgent returns the name of the first agent with no incoming edges.
func findSourceAgent(wf *WorkflowDefinition) string {
	hasIncoming := make(map[string]bool)
	for _, edge := range wf.Flow {
		hasIncoming[edge.To] = true
	}
	for _, a := range wf.Agents {
		if !hasIncoming[a.Name] {
			return a.Name
		}
	}
	return ""
}

// Validate implements the `waggle validate` command.
func Validate(_ context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("validate: workflow YAML file path required")
	}
	for _, path := range args {
		wf, err := LoadWorkflow(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "INVALID %s: %v\n", path, err)
			continue
		}
		fmt.Printf("OK %s (name=%q, agents=%d, edges=%d)\n",
			path, wf.Name, len(wf.Agents), len(wf.Flow))
	}
	return nil
}

// Dot implements the `waggle dot` command, exporting the DAG in Graphviz DOT format.
func Dot(_ context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("dot: workflow YAML file path required")
	}

	wf, err := LoadWorkflow(args[0])
	if err != nil {
		return fmt.Errorf("dot: %w", err)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "digraph %q {\n", wf.Name)
	sb.WriteString("  rankdir=LR;\n")
	sb.WriteString("  node [shape=box, style=filled, fillcolor=\"#1a1d27\", fontcolor=white, fontname=Helvetica];\n")
	sb.WriteString("  edge [color=\"#4a5568\"];\n\n")

	for _, a := range wf.Agents {
		label := a.Name
		if a.Type != "" {
			label += fmt.Sprintf("\\n[%s]", a.Type)
		}
		fmt.Fprintf(&sb, "  %q [label=%q];\n", a.Name, label)
	}
	sb.WriteString("\n")
	for _, edge := range wf.Flow {
		fmt.Fprintf(&sb, "  %q -> %q;\n", edge.From, edge.To)
	}
	sb.WriteString("}\n")

	fmt.Print(sb.String())
	return nil
}
