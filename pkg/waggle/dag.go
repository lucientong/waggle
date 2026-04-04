// Package waggle provides the core orchestration engine for Waggle.
// It manages Agent registration, DAG construction, topological scheduling,
// and concurrent execution via goroutines and channels.
package waggle

import (
	"errors"
	"fmt"
)

// ErrCycleDetected is returned when a cycle is found in the DAG.
var ErrCycleDetected = errors.New("cycle detected in DAG")

// node represents a single vertex in the DAG.
// It holds a type-erased agent (UntypedAgent) so that the DAG can store
// heterogeneous agent types. Typed agents are erased via agent.Erase before
// being added to the DAG.
type node struct {
	id   string
	name string
	// weight is the estimated execution cost used for critical path analysis.
	// Defaults to 1 if not set.
	weight int
}

// edge represents a directed data-flow connection from one node to another.
type edge struct {
	from string // source node id
	to   string // destination node id
}

// DAG is a directed acyclic graph representing an Agent workflow.
// Nodes are agents; edges represent data dependencies (output of from feeds input of to).
type DAG struct {
	nodes map[string]*node
	// adjacency stores outgoing edges: adjacency[id] = list of successor node ids.
	adjacency map[string][]string
	// reverse stores incoming edges: reverse[id] = list of predecessor node ids.
	reverse map[string][]string
	edges   []edge
}

// newDAG creates an empty DAG.
func newDAG() *DAG {
	return &DAG{
		nodes:     make(map[string]*node),
		adjacency: make(map[string][]string),
		reverse:   make(map[string][]string),
	}
}

// addNode inserts a node into the DAG. Returns an error if the id is already taken.
func (d *DAG) addNode(id, name string, weight int) error {
	if _, exists := d.nodes[id]; exists {
		return fmt.Errorf("node %q already exists in DAG", id)
	}
	if weight <= 0 {
		weight = 1
	}
	d.nodes[id] = &node{id: id, name: name, weight: weight}
	d.adjacency[id] = nil
	d.reverse[id] = nil
	return nil
}

// addEdge inserts a directed edge from -> to.
// Returns an error if either node does not exist, or if adding the edge would
// create a cycle (detected by DFS).
func (d *DAG) addEdge(from, to string) error {
	if _, ok := d.nodes[from]; !ok {
		return fmt.Errorf("source node %q not found", from)
	}
	if _, ok := d.nodes[to]; !ok {
		return fmt.Errorf("destination node %q not found", to)
	}
	if from == to {
		return fmt.Errorf("%w: self-loop on node %q", ErrCycleDetected, from)
	}

	// Check if adding this edge creates a cycle: detect a path from `to` back to `from`.
	if d.hasPath(to, from) {
		return fmt.Errorf("%w: adding edge %q -> %q would create a cycle", ErrCycleDetected, from, to)
	}

	d.adjacency[from] = append(d.adjacency[from], to)
	d.reverse[to] = append(d.reverse[to], from)
	d.edges = append(d.edges, edge{from: from, to: to})
	return nil
}

// hasPath returns true if there is a directed path from src to dst in the current DAG.
// Uses iterative DFS to avoid stack overflow on large graphs.
func (d *DAG) hasPath(src, dst string) bool {
	if src == dst {
		return true
	}
	visited := make(map[string]bool)
	stack := []string{src}

	for len(stack) > 0 {
		curr := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if curr == dst {
			return true
		}
		if visited[curr] {
			continue
		}
		visited[curr] = true

		for _, next := range d.adjacency[curr] {
			if !visited[next] {
				stack = append(stack, next)
			}
		}
	}
	return false
}

// TopologicalSort returns all node IDs in a valid topological order (Kahn's algorithm).
// Returns ErrCycleDetected if the graph contains a cycle (should not happen if addEdge
// is used correctly, but defensive check is included).
func (d *DAG) TopologicalSort() ([]string, error) {
	// Compute in-degree for each node.
	inDegree := make(map[string]int, len(d.nodes))
	for id := range d.nodes {
		inDegree[id] = len(d.reverse[id])
	}

	// Enqueue all source nodes (in-degree = 0).
	queue := make([]string, 0)
	for id, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, id)
		}
	}

	order := make([]string, 0, len(d.nodes))
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		order = append(order, curr)

		for _, next := range d.adjacency[curr] {
			inDegree[next]--
			if inDegree[next] == 0 {
				queue = append(queue, next)
			}
		}
	}

	if len(order) != len(d.nodes) {
		return nil, fmt.Errorf("%w: topological sort could not process all nodes", ErrCycleDetected)
	}
	return order, nil
}

// Layers returns nodes grouped by topological layer (BFS-based).
// All nodes in the same layer can be executed in parallel.
//
// Layer 0 contains source nodes (no predecessors).
// Layer N contains nodes whose all predecessors are in layers < N.
func (d *DAG) Layers() ([][]string, error) {
	// Compute in-degree for each node.
	inDegree := make(map[string]int, len(d.nodes))
	for id := range d.nodes {
		inDegree[id] = len(d.reverse[id])
	}

	// BFS layer-by-layer.
	var layers [][]string
	current := make([]string, 0)
	for id, deg := range inDegree {
		if deg == 0 {
			current = append(current, id)
		}
	}

	visited := 0
	for len(current) > 0 {
		layers = append(layers, current)
		visited += len(current)

		next := make([]string, 0)
		for _, id := range current {
			for _, succ := range d.adjacency[id] {
				inDegree[succ]--
				if inDegree[succ] == 0 {
					next = append(next, succ)
				}
			}
		}
		current = next
	}

	if visited != len(d.nodes) {
		return nil, fmt.Errorf("%w: layer computation could not process all nodes", ErrCycleDetected)
	}
	return layers, nil
}

// CriticalPath returns the sequence of node IDs that forms the longest weighted path
// through the DAG (the critical path). The critical path determines the minimum
// execution time for the entire workflow.
//
// Uses dynamic programming on a topological ordering.
func (d *DAG) CriticalPath() ([]string, int, error) {
	order, err := d.TopologicalSort()
	if err != nil {
		return nil, 0, err
	}

	// dist[id] = longest weighted path ending at node id (inclusive of id's weight).
	dist := make(map[string]int, len(d.nodes))
	prev := make(map[string]string, len(d.nodes)) // prev[id] = predecessor on critical path

	for _, id := range order {
		dist[id] = d.nodes[id].weight
		prev[id] = ""

		for _, predID := range d.reverse[id] {
			candidate := dist[predID] + d.nodes[id].weight
			if candidate > dist[id] {
				dist[id] = candidate
				prev[id] = predID
			}
		}
	}

	// Find the node with the maximum distance (end of critical path).
	maxDist := 0
	endNode := ""
	for id, d := range dist {
		if d > maxDist {
			maxDist = d
			endNode = id
		}
	}

	if endNode == "" {
		return nil, 0, nil
	}

	// Reconstruct path by following prev pointers.
	path := make([]string, 0)
	for curr := endNode; curr != ""; curr = prev[curr] {
		path = append([]string{curr}, path...)
	}

	return path, maxDist, nil
}

// Predecessors returns the IDs of all immediate predecessors of the given node.
func (d *DAG) Predecessors(id string) []string {
	return d.reverse[id]
}

// Successors returns the IDs of all immediate successors of the given node.
func (d *DAG) Successors(id string) []string {
	return d.adjacency[id]
}

// NodeCount returns the number of nodes in the DAG.
func (d *DAG) NodeCount() int {
	return len(d.nodes)
}

// EdgeCount returns the number of edges in the DAG.
func (d *DAG) EdgeCount() int {
	return len(d.edges)
}
