package git

import (
	"path/filepath"
	"strings"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// ComputeChurn uses go-git to walk commits and returns a map from short package
// path to file-change count over the last N days. Paths are relative to modPath
// within the repo at root.
func ComputeChurn(root string, days int, modPath string) map[string]int {
	if days <= 0 {
		return nil
	}

	repo, err := gogit.PlainOpen(root)
	if err != nil {
		return nil
	}

	since := time.Now().AddDate(0, 0, -days)
	iter, err := repo.Log(&gogit.LogOptions{Since: &since})
	if err != nil {
		return nil
	}

	absRoot, _ := filepath.Abs(root)
	var lines []string
	_ = iter.ForEach(func(c *object.Commit) error {
		files := commitChangedFiles(c)
		lines = append(lines, files...)
		return nil
	})

	return aggregateChurn(strings.Join(lines, "\n"), absRoot)
}

// aggregateChurn parses git log --name-only output and returns churn counts
// per directory. Test files (_test.go) are excluded — their churn is expected
// and should not inflate smell thresholds (Shotgun Surgery, Unstable Interface).
func aggregateChurn(gitOutput, absRoot string) map[string]int {
	result := make(map[string]int)
	for _, line := range strings.Split(gitOutput, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasSuffix(line, "_test.go") {
			continue
		}
		dir := filepath.Dir(line)
		if dir == "." {
			continue
		}
		full := filepath.Join(absRoot, dir)
		rel, err := filepath.Rel(absRoot, full)
		if err != nil {
			continue
		}
		rel = filepath.ToSlash(rel)
		result[rel]++
	}
	return result
}
