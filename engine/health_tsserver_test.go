package engine

import (
	"context"
	"testing"
)

func TestHealth_IncludesTypeScriptToolchain(t *testing.T) {
	eng := New(&mockStore{}, nil)
	h := eng.Health(context.Background())
	if h == nil {
		t.Fatal("nil HealthResult")
	}
	names := map[string]string{}
	for _, c := range h.Checks {
		names[c.Name] = c.Detail
	}
	if _, ok := names["typescript_language_server"]; !ok {
		t.Fatalf("missing typescript_language_server check; got %#v", names)
	}
	if _, ok := names["tsserver_path"]; !ok {
		t.Fatalf("missing tsserver_path check; got %#v", names)
	}
}
