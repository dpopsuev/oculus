//go:build integration

package testcontainer

import (
	"context"
	"testing"

	"github.com/dpopsuev/oculus/lang"
)

func TestContainerPool_Smoke(t *testing.T) {
	if err := Available(""); err != nil {
		t.Skipf("skipping: %v", err)
	}

	pool := NewPool("")
	defer pool.Shutdown(context.Background())

	// gopls should start and respond
	client, err := pool.Get(lang.Go, t.TempDir())
	if err != nil {
		t.Fatalf("Get(Go): %v", err)
	}
	if client == nil {
		t.Fatal("Get(Go) returned nil client")
	}

	status := pool.Status()
	if status.Active != 1 {
		t.Errorf("expected 1 active connection, got %d", status.Active)
	}
	t.Logf("pool status: %+v", status)
}
