package core

import (
	"errors"
	"testing"
)

func TestSentinelErrors(t *testing.T) {
	sentinels := []error{
		ErrTypeAnalyzerRequired,
		ErrDeepAnalyzerRequired,
		ErrNoTypesFound,
		ErrNoEntitiesFound,
		ErrNoInterfacesFound,
		ErrNoEntryProvided,
		ErrNoCallsFound,
		ErrUnknownDiagramType,
		ErrHexaRolesRequired,
	}
	for _, err := range sentinels {
		if err == nil {
			t.Error("sentinel error is nil")
		}
		if err.Error() == "" {
			t.Error("sentinel error has empty message")
		}
	}
	// Verify distinct errors
	for i, a := range sentinels {
		for j, b := range sentinels {
			if i != j && errors.Is(a, b) {
				t.Errorf("sentinel errors %d and %d are equal", i, j)
			}
		}
	}
}
