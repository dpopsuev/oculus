package engine

import (
	"context"
	"testing"
)

func TestBatchStr(t *testing.T) {
	a := BatchAction{Params: map[string]any{"key": "value"}}
	if got := batchStr(a, "key"); got != "value" {
		t.Errorf("batchStr(present) = %q, want %q", got, "value")
	}
	if got := batchStr(a, "missing"); got != "" {
		t.Errorf("batchStr(missing) = %q, want empty", got)
	}
	a2 := BatchAction{Params: map[string]any{"num": 42}}
	if got := batchStr(a2, "num"); got != "" {
		t.Errorf("batchStr(wrong type) = %q, want empty", got)
	}
}

func TestBatchInt(t *testing.T) {
	tests := []struct {
		name string
		val  any
		want int
	}{
		{"float64", float64(42), 42},
		{"int", 7, 7},
		{"string", "13", 13},
		{"invalid string", "abc", 0},
	}
	for _, tt := range tests {
		a := BatchAction{Params: map[string]any{"n": tt.val}}
		if got := batchInt(a, "n"); got != tt.want {
			t.Errorf("batchInt(%s) = %d, want %d", tt.name, got, tt.want)
		}
	}
	// Missing key
	a := BatchAction{Params: map[string]any{}}
	if got := batchInt(a, "n"); got != 0 {
		t.Errorf("batchInt(missing) = %d, want 0", got)
	}
}

func TestBatchStrSlice(t *testing.T) {
	// []string
	a1 := BatchAction{Params: map[string]any{"layers": []string{"a", "b"}}}
	got := batchStrSlice(a1, "layers")
	if len(got) != 2 || got[0] != "a" {
		t.Errorf("batchStrSlice([]string) = %v, want [a b]", got)
	}

	// []any
	a2 := BatchAction{Params: map[string]any{"layers": []any{"x", "y"}}}
	got2 := batchStrSlice(a2, "layers")
	if len(got2) != 2 || got2[0] != "x" {
		t.Errorf("batchStrSlice([]any) = %v, want [x y]", got2)
	}

	// Missing
	a3 := BatchAction{Params: map[string]any{}}
	if got3 := batchStrSlice(a3, "layers"); got3 != nil {
		t.Errorf("batchStrSlice(missing) = %v, want nil", got3)
	}
}

func TestBatchQuery_UnknownAction(t *testing.T) {
	store := newMockStore(testReport())
	eng := New(store, []string{"/tmp"})
	req := BatchRequest{
		Path: "/tmp",
		Actions: []BatchAction{
			{Name: "nonexistent_action", Params: map[string]any{}},
		},
	}
	resp, err := eng.BatchQuery(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Results))
	}
	if resp.Results[0].OK {
		t.Error("expected OK=false for unknown action")
	}
}

func TestBatchQuery_PathRequired(t *testing.T) {
	store := newMockStore(testReport())
	eng := New(store, []string{"/tmp"})
	req := BatchRequest{
		Path: "",
		Actions: []BatchAction{
			{Name: "deps", Params: map[string]any{"component": "core"}},
		},
	}
	_, err := eng.BatchQuery(context.Background(), req)
	if err == nil {
		t.Error("expected error for empty path")
	}
}
