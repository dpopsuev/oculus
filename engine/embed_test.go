package engine

import "testing"

func TestRRF(t *testing.T) {
	got := RRF([]string{"a", "b", "c"}, []string{"b", "a", "d"})
	if len(got) < 3 || got[0] != "a" && got[0] != "b" {
		t.Fatalf("RRF=%v", got)
	}
	// a and b should rank above c/d
	pos := map[string]int{}
	for i, id := range got {
		pos[id] = i
	}
	if pos["a"] > pos["c"] || pos["b"] > pos["d"] {
		t.Fatalf("unexpected order %v", got)
	}
}

func TestCosine(t *testing.T) {
	if cosine([]float32{1, 0}, []float32{1, 0}) < 0.99 {
		t.Fatal("identical vectors")
	}
	if cosine([]float32{1, 0}, []float32{0, 1}) > 0.01 {
		t.Fatal("orthogonal")
	}
}

func TestMarkDirty(t *testing.T) {
	eng := New(&mockStore{headSHA: "x"}, []string{"/tmp"})
	eng.MarkDirty("/tmp")
	if !eng.IsDirty("/tmp") {
		t.Fatal("expected dirty")
	}
	eng.ClearDirty("/tmp")
	if eng.IsDirty("/tmp") {
		t.Fatal("expected clear")
	}
}
