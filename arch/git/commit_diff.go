package git

import (
	"github.com/go-git/go-git/v5/plumbing/object"
)

// commitChangedFiles returns the list of file paths changed in a commit by
// comparing it to its first parent (or treating it as a root commit).
func commitChangedFiles(c *object.Commit) []string {
	tree, err := c.Tree()
	if err != nil {
		return nil
	}

	var parentTree *object.Tree
	if c.NumParents() > 0 {
		parent, err := c.Parents().Next()
		if err == nil {
			parentTree, _ = parent.Tree()
		}
	}

	changes, err := object.DiffTree(parentTree, tree)
	if err != nil {
		return nil
	}

	files := make([]string, 0, len(changes))
	for _, ch := range changes {
		name := ch.To.Name
		if name == "" {
			name = ch.From.Name
		}
		if name != "" {
			files = append(files, name)
		}
	}
	return files
}
