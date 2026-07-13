package lsp

import (
	"context"

	"github.com/dpopsuev/oculus/v3/lang"
)

// StubPool is a no-op pool for CLI mode. Get always returns ErrNoPool,
// forcing analyzers to fall back to cold-start per request.
type StubPool struct{}

func (s *StubPool) Get(lang.Language, string) (*Client, error)                    { return nil, ErrNoPool }
func (s *StubPool) Release(lang.Language, string)                                 {}
func (s *StubPool) References(context.Context, string, int, int) ([]Location, error) {
	return nil, ErrNoPool
}
func (s *StubPool) Definition(context.Context, string, int, int) ([]Location, error) {
	return nil, ErrNoPool
}
func (s *StubPool) DocumentSymbols(context.Context, string) ([]DocSymbol, error) {
	return nil, ErrNoPool
}
func (s *StubPool) PrepareRename(context.Context, string, int, int) (*PrepareResult, error) {
	return nil, ErrNoPool
}
func (s *StubPool) Rename(context.Context, string, int, int, string) (*WorkspaceEdit, error) {
	return nil, ErrNoPool
}
func (s *StubPool) MaxConcurrent(lang.Language) int                               { return 0 }
func (s *StubPool) Shutdown(context.Context) error                                { return nil }
func (s *StubPool) Status() PoolStatus                                            { return PoolStatus{} }
