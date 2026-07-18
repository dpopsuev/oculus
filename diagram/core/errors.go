package core

import "errors"

// Sentinel errors for diagram rendering.
var (
	ErrTypeAnalyzerRequired = errors.New("diagram requires a TypeAnalyzer")
	ErrDeepAnalyzerRequired = errors.New("diagram requires a DeepAnalyzer")
	ErrNoTypesFound         = errors.New("no types found for classes diagram")
	ErrNoEntitiesFound      = errors.New("no entities found for ER diagram")
	ErrNoInterfacesFound    = errors.New("no interfaces found for interfaces diagram")
	ErrNoEntryProvided      = errors.New("pass --entry <symbol> (no entry points auto-detected); e.g. --entry main")
	ErrNoCallsFound         = errors.New("no calls found from entry — try a different --entry")
	ErrUnknownDiagramType   = errors.New("unknown diagram type")
	ErrHexaRolesRequired    = errors.New("hexagonal classification required — run hexa_validate first")
	ErrSymbolGraphRequired  = errors.New("symbol graph required — run symbol_graph first")
)
