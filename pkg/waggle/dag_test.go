package waggle

import (
	"errors"
	"testing"
)

// TestDAG_AddNode verifies basic node insertion.
func TestDAG_AddNode(t *testing.T) {
	d := newDAG()
	if err := d.addNode("a", "Agent-A", 1); err != nil {
		t.Fatalf("addNode() unexpected error: %v", err)
	}
	if d.NodeCount() != 1 {
		t.Errorf("NodeCount() = %d, want 1", d.NodeCount())
	}
}

// TestDAG_AddNode_Duplicate verifies that duplicate node IDs are rejected.
func TestDAG_AddNode_Duplicate(t *testing.T) {
	d := newDAG()
	d.addNode("a", "A", 1) //nolint
	if err := d.addNode("a", "A2", 1); err == nil {
		t.Error("addNode() expected error for duplicate id, got nil")
	}
}

// TestDAG_AddEdge_Basic verifies a simple edge is accepted.
func TestDAG_AddEdge_Basic(t *testing.T) {
	d := newDAG()
	d.addNode("a", "A", 1) //nolint
	d.addNode("b", "B", 1) //nolint
	if err := d.addEdge("a", "b"); err != nil {
		t.Fatalf("addEdge() unexpected error: %v", err)
	}
	if d.EdgeCount() != 1 {
		t.Errorf("EdgeCount() = %d, want 1", d.EdgeCount())
	}
}

// TestDAG_AddEdge_SelfLoop verifies that self-loops are rejected.
func TestDAG_AddEdge_SelfLoop(t *testing.T) {
	d := newDAG()
	d.addNode("a", "A", 1) //nolint
	err := d.addEdge("a", "a")
	if err == nil {
		t.Fatal("addEdge() expected cycle error for self-loop, got nil")
	}
	if !errors.Is(err, ErrCycleDetected) {
		t.Errorf("error = %v, want ErrCycleDetected", err)
	}
}

// TestDAG_AddEdge_Cycle verifies that edges creating a cycle are rejected.
func TestDAG_AddEdge_Cycle(t *testing.T) {
	d := newDAG()
	d.addNode("a", "A", 1) //nolint
	d.addNode("b", "B", 1) //nolint
	d.addNode("c", "C", 1) //nolint
	d.addEdge("a", "b")    //nolint
	d.addEdge("b", "c")    //nolint

	// Adding c -> a would create a cycle: a -> b -> c -> a
	err := d.addEdge("c", "a")
	if err == nil {
		t.Fatal("addEdge() expected cycle error, got nil")
	}
	if !errors.Is(err, ErrCycleDetected) {
		t.Errorf("error = %v, want ErrCycleDetected", err)
	}
}

// TestDAG_AddEdge_UnknownNode verifies that referencing nonexistent nodes fails.
func TestDAG_AddEdge_UnknownNode(t *testing.T) {
	d := newDAG()
	d.addNode("a", "A", 1) //nolint
	if err := d.addEdge("a", "z"); err == nil {
		t.Error("addEdge() expected error for unknown destination, got nil")
	}
	if err := d.addEdge("z", "a"); err == nil {
		t.Error("addEdge() expected error for unknown source, got nil")
	}
}

// TestDAG_TopologicalSort_Linear verifies sort order for a linear chain.
func TestDAG_TopologicalSort_Linear(t *testing.T) {
	d := newDAG()
	for _, id := range []string{"a", "b", "c", "d"} {
		d.addNode(id, id, 1) //nolint
	}
	d.addEdge("a", "b") //nolint
	d.addEdge("b", "c") //nolint
	d.addEdge("c", "d") //nolint

	order, err := d.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort() unexpected error: %v", err)
	}
	if len(order) != 4 {
		t.Fatalf("TopologicalSort() returned %d nodes, want 4", len(order))
	}

	// Verify ordering: a must appear before b, b before c, c before d.
	pos := make(map[string]int)
	for i, id := range order {
		pos[id] = i
	}
	checks := [][2]string{{"a", "b"}, {"b", "c"}, {"c", "d"}}
	for _, pair := range checks {
		if pos[pair[0]] >= pos[pair[1]] {
			t.Errorf("expected %q before %q in order %v", pair[0], pair[1], order)
		}
	}
}

// TestDAG_TopologicalSort_Diamond verifies sort for a diamond-shaped DAG.
func TestDAG_TopologicalSort_Diamond(t *testing.T) {
	//   a
	//  / \
	// b   c
	//  \ /
	//   d
	d := newDAG()
	for _, id := range []string{"a", "b", "c", "d"} {
		d.addNode(id, id, 1) //nolint
	}
	d.addEdge("a", "b") //nolint
	d.addEdge("a", "c") //nolint
	d.addEdge("b", "d") //nolint
	d.addEdge("c", "d") //nolint

	order, err := d.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort() unexpected error: %v", err)
	}

	pos := make(map[string]int)
	for i, id := range order {
		pos[id] = i
	}
	if pos["a"] >= pos["b"] || pos["a"] >= pos["c"] {
		t.Errorf("a must come before b and c; order=%v", order)
	}
	if pos["b"] >= pos["d"] || pos["c"] >= pos["d"] {
		t.Errorf("b and c must come before d; order=%v", order)
	}
}

// TestDAG_Layers_Linear verifies that a linear chain produces one node per layer.
func TestDAG_Layers_Linear(t *testing.T) {
	d := newDAG()
	for _, id := range []string{"a", "b", "c"} {
		d.addNode(id, id, 1) //nolint
	}
	d.addEdge("a", "b") //nolint
	d.addEdge("b", "c") //nolint

	layers, err := d.Layers()
	if err != nil {
		t.Fatalf("Layers() unexpected error: %v", err)
	}
	if len(layers) != 3 {
		t.Errorf("Layers() returned %d layers, want 3; layers=%v", len(layers), layers)
	}
	for i, layer := range layers {
		if len(layer) != 1 {
			t.Errorf("layer %d has %d nodes, want 1", i, len(layer))
		}
	}
}

// TestDAG_Layers_Parallel verifies parallel nodes appear in the same layer.
func TestDAG_Layers_Parallel(t *testing.T) {
	//   a
	//  /|\
	// b c d   <- all in layer 1 (parallel)
	d := newDAG()
	for _, id := range []string{"a", "b", "c", "d"} {
		d.addNode(id, id, 1) //nolint
	}
	d.addEdge("a", "b") //nolint
	d.addEdge("a", "c") //nolint
	d.addEdge("a", "d") //nolint

	layers, err := d.Layers()
	if err != nil {
		t.Fatalf("Layers() unexpected error: %v", err)
	}
	if len(layers) != 2 {
		t.Errorf("Layers() returned %d layers, want 2; layers=%v", len(layers), layers)
	}
	if len(layers[0]) != 1 {
		t.Errorf("layer 0 should have 1 node (a), got %d", len(layers[0]))
	}
	if len(layers[1]) != 3 {
		t.Errorf("layer 1 should have 3 nodes (b,c,d), got %d", len(layers[1]))
	}
}

// TestDAG_CriticalPath_Linear verifies critical path for a linear chain.
func TestDAG_CriticalPath_Linear(t *testing.T) {
	d := newDAG()
	d.addNode("a", "A", 1) //nolint
	d.addNode("b", "B", 2) //nolint
	d.addNode("c", "C", 3) //nolint
	d.addEdge("a", "b")    //nolint
	d.addEdge("b", "c")    //nolint

	path, cost, err := d.CriticalPath()
	if err != nil {
		t.Fatalf("CriticalPath() unexpected error: %v", err)
	}
	if cost != 6 { // 1 + 2 + 3
		t.Errorf("CriticalPath() cost = %d, want 6", cost)
	}
	if len(path) != 3 {
		t.Errorf("CriticalPath() path length = %d, want 3; path=%v", len(path), path)
	}
}

// TestDAG_CriticalPath_Diamond verifies critical path picks the heavier branch.
func TestDAG_CriticalPath_Diamond(t *testing.T) {
	//   a(1)
	//  /    \
	// b(10)  c(1)
	//  \    /
	//   d(1)
	// Critical path: a -> b -> d = 12
	d := newDAG()
	d.addNode("a", "A", 1)  //nolint
	d.addNode("b", "B", 10) //nolint
	d.addNode("c", "C", 1)  //nolint
	d.addNode("d", "D", 1)  //nolint
	d.addEdge("a", "b")     //nolint
	d.addEdge("a", "c")     //nolint
	d.addEdge("b", "d")     //nolint
	d.addEdge("c", "d")     //nolint

	path, cost, err := d.CriticalPath()
	if err != nil {
		t.Fatalf("CriticalPath() unexpected error: %v", err)
	}
	if cost != 12 {
		t.Errorf("CriticalPath() cost = %d, want 12", cost)
	}
	// Path should include b (the heavy node).
	found := false
	for _, id := range path {
		if id == "b" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("CriticalPath() path %v should include node 'b'", path)
	}
}

// TestDAG_Predecessors_Successors verifies adjacency queries.
func TestDAG_Predecessors_Successors(t *testing.T) {
	d := newDAG()
	d.addNode("a", "A", 1) //nolint
	d.addNode("b", "B", 1) //nolint
	d.addNode("c", "C", 1) //nolint
	d.addEdge("a", "b")    //nolint
	d.addEdge("a", "c")    //nolint

	succs := d.Successors("a")
	if len(succs) != 2 {
		t.Errorf("Successors(a) = %v, want 2 elements", succs)
	}

	preds := d.Predecessors("b")
	if len(preds) != 1 || preds[0] != "a" {
		t.Errorf("Predecessors(b) = %v, want [a]", preds)
	}
}

// TestDAG_Empty verifies an empty DAG is valid.
func TestDAG_Empty(t *testing.T) {
	d := newDAG()
	order, err := d.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort() on empty DAG unexpected error: %v", err)
	}
	if len(order) != 0 {
		t.Errorf("expected empty order, got %v", order)
	}

	layers, err := d.Layers()
	if err != nil {
		t.Fatalf("Layers() on empty DAG unexpected error: %v", err)
	}
	if len(layers) != 0 {
		t.Errorf("expected empty layers, got %v", layers)
	}
}
