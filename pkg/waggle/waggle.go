package waggle

import (
	"context"
	"fmt"

	"github.com/lucientong/waggle/pkg/agent"
)

// Waggle is the core orchestration engine.
//
// It manages a DAG of agents, provides methods to build the execution graph,
// and runs the workflow by scheduling agents according to their topological order.
//
// Usage:
//
//	w := waggle.New()
//
//	fetcher   := agent.Func[string, Document]("fetcher", fetchDoc)
//	summarizer := agent.Func[Document, string]("summarizer", summarize)
//
//	// Register agents and declare data-flow edges.
//	w.Register(agent.Erase(fetcher), agent.Erase(summarizer))
//	w.Connect("fetcher", "summarizer")
//
//	// Run the workflow.
//	result, err := w.RunFrom(ctx, "fetcher", "https://example.com")
type Waggle struct {
	dag    *DAG
	agents map[string]agent.UntypedAgent
}

// New creates a new empty Waggle orchestrator.
func New() *Waggle {
	return &Waggle{
		dag:    newDAG(),
		agents: make(map[string]agent.UntypedAgent),
	}
}

// Register adds one or more type-erased agents to the orchestrator.
// Each agent's Name() is used as the node ID in the DAG.
//
// Agents must be registered before Connect can reference them.
func (w *Waggle) Register(agents ...agent.UntypedAgent) error {
	for _, a := range agents {
		id := a.Name()
		if err := w.dag.addNode(id, id, 1); err != nil {
			return fmt.Errorf("waggle: register agent %q: %w", id, err)
		}
		w.agents[id] = a
	}
	return nil
}

// Connect declares a data-flow dependency: the output of the `from` agent is
// passed as input to the `to` agent. Both agents must be registered first.
//
// Connect returns ErrCycleDetected if the edge would introduce a cycle.
func (w *Waggle) Connect(from, to string) error {
	if _, ok := w.agents[from]; !ok {
		return fmt.Errorf("waggle: connect: agent %q not registered", from)
	}
	if _, ok := w.agents[to]; !ok {
		return fmt.Errorf("waggle: connect: agent %q not registered", to)
	}
	return w.dag.addEdge(from, to)
}

// RunFrom executes the workflow starting from the agent identified by startID.
// The provided input is passed to the start agent; subsequent agents receive the
// output of their single predecessor.
//
// RunFrom only supports linear pipelines (each node has at most one predecessor and
// one successor). For fan-out/fan-in topologies, use the patterns package.
//
// Returns the output of the final sink node.
func (w *Waggle) RunFrom(ctx context.Context, startID string, input any) (any, error) {
	order, err := w.dag.TopologicalSort()
	if err != nil {
		return nil, fmt.Errorf("waggle: topological sort failed: %w", err)
	}

	// Find the index of the start node.
	startIdx := -1
	for i, id := range order {
		if id == startID {
			startIdx = i
			break
		}
	}
	if startIdx < 0 {
		return nil, fmt.Errorf("waggle: start agent %q not found in topological order", startID)
	}

	// Execute agents in order, passing output from one to the next.
	current := input
	for _, id := range order[startIdx:] {
		a, ok := w.agents[id]
		if !ok {
			return nil, fmt.Errorf("waggle: agent %q not found", id)
		}

		if err := ctx.Err(); err != nil {
			return nil, err
		}

		current, err = a.RunUntyped(ctx, current)
		if err != nil {
			return nil, fmt.Errorf("waggle: agent %q failed: %w", id, err)
		}
	}

	return current, nil
}

// DAGInfo returns a read-only snapshot of the DAG structure for inspection.
// Useful for visualization and validation.
func (w *Waggle) DAGInfo() DAGSnapshot {
	nodes := make([]NodeInfo, 0, len(w.agents))
	for id, a := range w.agents {
		nodes = append(nodes, NodeInfo{
			ID:           id,
			Name:         a.Name(),
			Predecessors: w.dag.Predecessors(id),
			Successors:   w.dag.Successors(id),
		})
	}
	return DAGSnapshot{
		Nodes:     nodes,
		NodeCount: w.dag.NodeCount(),
		EdgeCount: w.dag.EdgeCount(),
	}
}

// DAGSnapshot is a read-only view of the DAG structure.
type DAGSnapshot struct {
	Nodes     []NodeInfo
	NodeCount int
	EdgeCount int
}

// NodeInfo holds basic information about a DAG node.
type NodeInfo struct {
	ID           string
	Name         string
	Predecessors []string
	Successors   []string
}
