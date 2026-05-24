package lsp

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"
)

// DiagnosticSeverity mirrors the LSP DiagnosticSeverity enum.
type DiagnosticSeverity int

const (
	SeverityError       DiagnosticSeverity = 1
	SeverityWarning     DiagnosticSeverity = 2
	SeverityInformation DiagnosticSeverity = 3
	SeverityHint        DiagnosticSeverity = 4
)

// Diagnostic is a single LSP diagnostic item.
type Diagnostic struct {
	Range    DiagRange          `json:"range"`
	Severity DiagnosticSeverity `json:"severity,omitempty"`
	Code     any                `json:"code,omitempty"`
	Source   string             `json:"source,omitempty"`
	Message  string             `json:"message"`
}

// DiagRange is a start/end position in a text document.
type DiagRange struct {
	Start DiagPosition `json:"start"`
	End   DiagPosition `json:"end"`
}

// DiagPosition is a zero-based line/character offset.
type DiagPosition struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// publishDiagnosticsParams mirrors textDocument/publishDiagnostics params.
type publishDiagnosticsParams struct {
	URI         string       `json:"uri"`
	Version     *int         `json:"version,omitempty"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// VersionedDiagnosticMap stores LSP diagnostics keyed by file URI.
// Each write atomically bumps a version counter so callers can detect
// changes with O(1) cost — no map copy required.
type VersionedDiagnosticMap struct {
	mu      sync.RWMutex
	data    map[string][]Diagnostic
	version atomic.Uint64
}

// newVersionedDiagnosticMap creates an empty map.
func newVersionedDiagnosticMap() *VersionedDiagnosticMap {
	return &VersionedDiagnosticMap{data: make(map[string][]Diagnostic)}
}

// Set replaces the diagnostic list for uri and bumps the version.
func (m *VersionedDiagnosticMap) Set(uri string, diags []Diagnostic) {
	m.mu.Lock()
	m.data[uri] = diags
	m.mu.Unlock()
	m.version.Add(1)
}

// Get returns the diagnostic list for uri and whether it exists.
func (m *VersionedDiagnosticMap) Get(uri string) ([]Diagnostic, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	diags, ok := m.data[uri]
	return diags, ok
}

// All returns a shallow copy of the full map.
func (m *VersionedDiagnosticMap) All() map[string][]Diagnostic {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string][]Diagnostic, len(m.data))
	for k, v := range m.data {
		out[k] = v
	}
	return out
}

// Version returns the current monotonic version counter.
func (m *VersionedDiagnosticMap) Version() uint64 {
	return m.version.Load()
}

// WaitForDiagnostics blocks until diagnostics stop changing for a settling
// period, or until timeout or ctx is cancelled. Uses a two-phase algorithm:
//
//  1. Wait up to firstChangeDuration for the version to change at all. If no
//     change arrives the server is not publishing — return early.
//  2. Once the first change arrives, wait until the version is stable for
//     settleDuration before returning.
func (m *VersionedDiagnosticMap) WaitForDiagnostics(ctx context.Context, timeout time.Duration) {
	const (
		firstChangeDuration = 1 * time.Second
		settleDuration      = 300 * time.Millisecond
		pollFast            = 50 * time.Millisecond
		pollSlow            = 100 * time.Millisecond
	)

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	firstChange := time.NewTimer(min(timeout, firstChangeDuration))
	defer firstChange.Stop()

	prev := m.Version()
	ticker := time.NewTicker(pollSlow)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-deadline.C:
			return
		case <-firstChange.C:
			return // no change — server won't publish
		case <-ticker.C:
			cur := m.Version()
			if cur != prev {
				m.waitForSettle(ctx, deadline.C, settleDuration, pollFast)
				return
			}
		}
	}
}

// waitForSettle polls until version is stable for settleDuration.
func (m *VersionedDiagnosticMap) waitForSettle(ctx context.Context, deadline <-chan time.Time, settle, poll time.Duration) {
	last := m.Version()
	stableAt := time.Now()

	ticker := time.NewTicker(poll)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-deadline:
			return
		case <-ticker.C:
			cur := m.Version()
			if cur != last {
				last = cur
				stableAt = time.Now()
			} else if time.Since(stableAt) >= settle {
				return
			}
		}
	}
}

// handlePublishDiagnostics decodes a textDocument/publishDiagnostics
// notification payload and updates the map.
func (m *VersionedDiagnosticMap) handlePublishDiagnostics(params json.RawMessage) {
	var p publishDiagnosticsParams
	if err := json.Unmarshal(params, &p); err != nil {
		return
	}
	m.Set(p.URI, p.Diagnostics)
}
