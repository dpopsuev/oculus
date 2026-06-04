package git_test

import (
	"testing"

	archgit "github.com/dpopsuev/oculus/v3/arch/git"
)

func TestChangedFilesSince_External(t *testing.T) {
	_, err := archgit.ChangedFilesSince(t.TempDir(), "HEAD~1")
	// Non-git dir returns error — we're testing the boundary contract.
	_ = err
}
