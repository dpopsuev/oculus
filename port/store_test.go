package port

import "testing"

func TestNewDesiredState(t *testing.T) {
	ds := NewDesiredState("domain", "application", "infrastructure")
	if ds == nil {
		t.Fatal("NewDesiredState returned nil")
	}
	if len(ds.Layers) != 3 {
		t.Errorf("got %d layers, want 3", len(ds.Layers))
	}
	if ds.Layers[0] != "domain" {
		t.Errorf("first layer = %q, want %q", ds.Layers[0], "domain")
	}
}

func TestNewDesiredStateEmpty(t *testing.T) {
	ds := NewDesiredState()
	if ds == nil {
		t.Fatal("NewDesiredState returned nil")
	}
	if len(ds.Layers) != 0 {
		t.Errorf("got %d layers, want 0", len(ds.Layers))
	}
}

func TestDesiredStateFields(t *testing.T) {
	ds := &DesiredState{
		Layers: []string{"core", "infra"},
		Boundaries: []BoundaryRule{
			{FromPattern: "infra/*", ToPattern: "core/*", Allow: false},
		},
		Constraints: []HealthConstraint{
			{Component: "core", MaxFanIn: 5, MaxChurn: 10},
		},
		Roles: map[string]string{"core": "domain", "infra": "infrastructure"},
		Accepted: []AcceptedViolation{
			{Component: "legacy", Principle: "SRP", Reason: "planned for refactor"},
		},
	}
	if len(ds.Boundaries) != 1 {
		t.Errorf("got %d boundaries, want 1", len(ds.Boundaries))
	}
	if ds.Boundaries[0].Allow {
		t.Error("boundary should deny, not allow")
	}
	if ds.Constraints[0].MaxFanIn != 5 {
		t.Errorf("MaxFanIn = %d, want 5", ds.Constraints[0].MaxFanIn)
	}
}

func TestCallerSiteFields(t *testing.T) {
	cs := CallerSite{
		Caller:       "Run",
		CallerPkg:    "internal/core",
		Line:         42,
		File:         "core.go",
		ReceiverType: "Service",
	}
	if cs.Caller != "Run" {
		t.Errorf("Caller = %q, want %q", cs.Caller, "Run")
	}
	if cs.Line != 42 {
		t.Errorf("Line = %d, want 42", cs.Line)
	}
}

func TestHistoryEntry(t *testing.T) {
	he := HistoryEntry{
		SHA:        "abc123",
		Source:     "scan_local",
		RepoPath:   "/tmp/repo",
		Components: 10,
		Edges:      15,
	}
	if he.Components != 10 {
		t.Errorf("Components = %d, want 10", he.Components)
	}
}

func TestProjectInfo(t *testing.T) {
	pi := ProjectInfo{
		Path:       "/tmp/repo",
		Name:       "test-project",
		Language:   "go",
		LastSHA:    "abc123",
		Components: 5,
	}
	if pi.Name != "test-project" {
		t.Errorf("Name = %q, want %q", pi.Name, "test-project")
	}
}

func TestComponentMeta(t *testing.T) {
	cm := ComponentMeta{
		Name:        "engine",
		Role:        "facade",
		Keywords:    []string{"analysis", "scan"},
		Description: "Main engine facade",
		Layer:       0,
		Health:      "healthy",
		LOC:         2500,
		FanIn:       8,
	}
	if cm.LOC != 2500 {
		t.Errorf("LOC = %d, want 2500", cm.LOC)
	}
	if len(cm.Keywords) != 2 {
		t.Errorf("Keywords len = %d, want 2", len(cm.Keywords))
	}
}
