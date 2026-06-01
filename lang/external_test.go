package lang_test

import (
	"testing"

	"github.com/dpopsuev/oculus/v3/lang"
)

func TestDetectLanguage_External(t *testing.T) {
	detected := lang.DetectLanguage(t.TempDir())
	_ = detected
}

// --- OCL-BUG-12: clangd must have job-count cap in Args ---

// TestClangdServerEntry_HasJobCap reproduces OCL-BUG-12.
// clangd defaults to using all CPUs for background indexing when spawned
// without --j. On a 16-core machine this committed 88GB and caused load 60.
//
// Given the clangd ServerEntry from the registry
// When Args are inspected
// Then --j=N and --background-index-priority=background are both present
func TestClangdServerEntry_HasJobCap(t *testing.T) {
	entry := lang.DefaultServerEntry(lang.Cpp)
	if entry == nil {
		t.Skip("no clangd registry entry")
	}

	hasJobFlag := false
	hasPriorityFlag := false
	for _, arg := range entry.Args {
		if len(arg) >= 3 && arg[:3] == "--j" {
			hasJobFlag = true
		}
		if arg == "--background-index-priority=background" {
			hasPriorityFlag = true
		}
	}

	if !hasJobFlag {
		t.Errorf("clangd Args missing --j=N; current args: %v — OCL-BUG-12: clangd defaults to all CPUs", entry.Args)
	}
	if !hasPriorityFlag {
		t.Errorf("clangd Args missing --background-index-priority=background; current args: %v — OCL-BUG-12", entry.Args)
	}
}
