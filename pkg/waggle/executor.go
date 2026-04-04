package waggle

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/lucientong/waggle/pkg/agent"
)

// result holds the output of a single agent execution.
type result struct {
	nodeID string
	value  any
	err    error
}

// Executor runs a DAG of untyped agents concurrently.
//
// The execution model:
//   - Agents with no predecessors (source nodes) are started immediately.
//   - An agent is started only when all of its predecessors have completed successfully.
//   - A single predecessor failure cancels the context, causing all running agents to stop.
//   - The final result is collected from the single sink node.
//
// This executor supports fan-out (one agent feeds multiple successors) and
// fan-in (multiple predecessors feed one agent) topologies, as used by the
// Parallel, Race, Vote, Router, and Loop patterns.
type Executor struct {
	dag    *DAG
	agents map[string]agent.UntypedAgent
}

// newExecutor creates a new Executor for the given DAG and agent map.
func newExecutor(dag *DAG, agents map[string]agent.UntypedAgent) *Executor {
	return &Executor{dag: dag, agents: agents}
}

// Run executes the DAG starting from source nodes with the given input.
//
// For DAGs with multiple source nodes, the input is broadcast to all sources.
// For DAGs with multiple sink nodes, only the result of the last node in
// topological order is returned; use patterns.Parallel for explicit fan-out.
//
// Returns the output of the sink node (single output DAG) or an error if any
// agent fails. On error, in-progress agents are cancelled via context propagation.
func (e *Executor) Run(ctx context.Context, input any) (any, error) {
	order, err := e.dag.TopologicalSort()
	if err != nil {
		return nil, fmt.Errorf("executor: %w", err)
	}
	if len(order) == 0 {
		return input, nil
	}

	// outputs stores the completed output of each agent.
	var mu sync.Mutex
	outputs := make(map[string]any, len(order))

	// Use a cancellable context so that a single failure stops all goroutines.
	execCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// errCh collects the first error.
	errCh := make(chan error, 1)

	// ready tracks how many predecessors have completed for each node.
	readyCount := make(map[string]int, len(order))
	for _, id := range order {
		readyCount[id] = 0
	}

	// trigger fires when a node's predecessor count reaches inDegree.
	// We use a channel per node to signal readiness.
	triggers := make(map[string]chan struct{}, len(order))
	for _, id := range order {
		inDeg := len(e.dag.Predecessors(id))
		if inDeg == 0 {
			ch := make(chan struct{}, 1)
			ch <- struct{}{} // source nodes are immediately ready
			triggers[id] = ch
		} else {
			triggers[id] = make(chan struct{}, 1)
		}
	}

	var wg sync.WaitGroup

	// Launch a goroutine for each node. Each goroutine waits until its trigger
	// fires (all predecessors done), then executes the agent.
	for _, nodeID := range order {
		id := nodeID

		wg.Add(1)
		go func() {
			defer wg.Done()

			// Wait for readiness signal or context cancellation.
			select {
			case <-execCtx.Done():
				return
			case <-triggers[id]:
			}

			// If context was already cancelled when we woke up, bail out.
			if execCtx.Err() != nil {
				return
			}

			// Determine input for this node.
			var nodeInput any
			preds := e.dag.Predecessors(id)
			if len(preds) == 0 {
				nodeInput = input
			} else if len(preds) == 1 {
				mu.Lock()
				nodeInput = outputs[preds[0]]
				mu.Unlock()
			} else {
				// Fan-in: collect all predecessor outputs as a slice.
				// The agent receiving fan-in input must handle []any.
				fanIn := make([]any, len(preds))
				mu.Lock()
				for i, predID := range preds {
					fanIn[i] = outputs[predID]
				}
				mu.Unlock()
				nodeInput = fanIn
			}

			a := e.agents[id]
			start := time.Now()
			slog.Debug("agent start", "agent", id)

			out, err := a.RunUntyped(execCtx, nodeInput)
			duration := time.Since(start)

			if err != nil {
				slog.Error("agent failed", "agent", id, "duration", duration, "error", err)
				// Cancel the whole execution and send the error (non-blocking).
				cancel()
				select {
				case errCh <- fmt.Errorf("agent %q: %w", id, err):
				default:
				}
				return
			}

			slog.Debug("agent done", "agent", id, "duration", duration)

			// Store the output and signal successors.
			mu.Lock()
			outputs[id] = out
			mu.Unlock()

			for _, succID := range e.dag.Successors(id) {
				mu.Lock()
				readyCount[succID]++
				ready := readyCount[succID] == len(e.dag.Predecessors(succID))
				mu.Unlock()

				if ready {
					select {
					case triggers[succID] <- struct{}{}:
					default:
					}
				}
			}
		}()
	}

	// Wait for all goroutines to finish.
	wg.Wait()

	// Check for errors first.
	select {
	case err := <-errCh:
		return nil, err
	default:
	}

	// Return the output of the last node in topological order (the sink).
	sinkID := order[len(order)-1]
	mu.Lock()
	out := outputs[sinkID]
	mu.Unlock()

	return out, nil
}
