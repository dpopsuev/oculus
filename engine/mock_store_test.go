package engine

import (
	"context"

	"github.com/dpopsuev/oculus/arch"
	"github.com/dpopsuev/oculus/port"
)

// mockStore implements engine.Store for unit testing Engine methods.
// Set reportHit=true to make getOrScan return the canned report
// without triggering real filesystem scans.
type mockStore struct {
	report       *arch.ContextReport
	reportHit    bool
	reportErr    error
	headSHA      string
	desiredState *port.DesiredState
	desiredErr   error
	projects     []port.ProjectInfo
	components   []port.ComponentMeta
	history      []port.HistoryEntry

	// SHA-specific reports for GetScanDiff tests.
	reportsBySHA map[string]*arch.ContextReport

	// Call counters for write assertions.
	putReportCalls  int
	putMetaCalls    int
	recordScanCalls int
	invalidateCalls int
}

func newMockStore(report *arch.ContextReport) *mockStore {
	return &mockStore{
		report:    report,
		reportHit: true,
		headSHA:   "deadbeef",
	}
}

// --- port.ReportStore ---

func (m *mockStore) GetReport(_ context.Context, _, sha string) (*arch.ContextReport, bool, error) {
	if m.reportsBySHA != nil {
		if r, ok := m.reportsBySHA[sha]; ok {
			return r, true, nil
		}
	}
	return m.report, m.reportHit, m.reportErr
}

func (m *mockStore) PutReport(_ context.Context, _, _ string, _ *arch.ContextReport) error {
	m.putReportCalls++
	return nil
}

func (m *mockStore) Invalidate(_ context.Context, _ string) error {
	m.invalidateCalls++
	return nil
}

// --- port.HistoryStore ---

func (m *mockStore) RecordScan(_ context.Context, _, _, _ string, _ *arch.ContextReport) error {
	m.recordScanCalls++
	return nil
}

func (m *mockStore) ListHistory(_ context.Context, _ string, _ int) ([]port.HistoryEntry, error) {
	return m.history, nil
}

func (m *mockStore) GetHistoryReport(_ context.Context, _ string, _ int) (*arch.ContextReport, error) {
	return m.report, nil
}

// --- port.GitResolver ---

func (m *mockStore) ResolveHEAD(_ string) string {
	if m.headSHA != "" {
		return m.headSHA
	}
	return "deadbeef"
}

func (m *mockStore) ResolveBranch(_, _ string) (string, error) {
	return m.headSHA, nil
}

// --- port.DesiredStateStore ---

func (m *mockStore) GetDesiredState(_ context.Context, _ string) (*port.DesiredState, error) {
	return m.desiredState, m.desiredErr
}

func (m *mockStore) PutDesiredState(_ context.Context, _ string, _ *port.DesiredState) error {
	return nil
}

// --- Extra Store methods ---

func (m *mockStore) PutComponentMeta(_ context.Context, _, _ string, _ []port.ComponentMeta) error {
	m.putMetaCalls++
	return nil
}

func (m *mockStore) SearchComponents(_ context.Context, _, _, _ string) ([]port.ComponentMeta, error) {
	return m.components, nil
}

func (m *mockStore) ListProjects(_ context.Context) ([]port.ProjectInfo, error) {
	return m.projects, nil
}
