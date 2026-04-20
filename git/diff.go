// Package git provides git operations used across Locus packages.
package git

import (
	"fmt"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// ChangedFilesSince returns the list of changed files since a git ref.
func ChangedFilesSince(repoPath, since string) ([]string, error) {
	repo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		return nil, fmt.Errorf("open repo: %w", err)
	}

	sinceHash, err := repo.ResolveRevision(plumbing.Revision(since))
	if err != nil {
		return nil, fmt.Errorf("resolve %q: %w", since, err)
	}

	head, err := repo.Head()
	if err != nil {
		return nil, fmt.Errorf("head: %w", err)
	}

	sinceCommit, err := repo.CommitObject(*sinceHash)
	if err != nil {
		return nil, fmt.Errorf("commit %s: %w", sinceHash, err)
	}
	headCommit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return nil, fmt.Errorf("commit %s: %w", head.Hash(), err)
	}

	sinceTree, err := sinceCommit.Tree()
	if err != nil {
		return nil, err
	}
	headTree, err := headCommit.Tree()
	if err != nil {
		return nil, err
	}

	changes, err := object.DiffTree(sinceTree, headTree)
	if err != nil {
		return nil, err
	}

	var files []string
	for _, c := range changes {
		name := c.To.Name
		if name == "" {
			name = c.From.Name
		}
		files = append(files, name)
	}
	return files, nil
}
