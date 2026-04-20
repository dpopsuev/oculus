package git

import (
	"path/filepath"
	"sort"
	"strings"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

const (
	// MaxAuthorsPerPackage is the max authors returned per package.
	MaxAuthorsPerPackage = 5
	// MaxFileHotSpots is the max file hotspots returned.
	MaxFileHotSpots = 50
)

// PackageCommit represents a single git commit associated with a package.
type PackageCommit struct {
	Package string    `json:"package"`
	Hash    string    `json:"hash"`
	Author  string    `json:"author"`
	Date    time.Time `json:"date"`
	Message string    `json:"message"`
}

// Author represents a contributor with their commit count.
type Author struct {
	Name    string `json:"name"`
	Commits int    `json:"commits"`
}

// HotFile identifies a frequently-changed individual file.
type HotFile struct {
	Path    string `json:"path"`
	Package string `json:"package"`
	Changes int    `json:"changes"`
}

// RecentCommits returns per-package recent commits from git history.
func RecentCommits(root string, days int, modPath string) []PackageCommit {
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
	var commits []PackageCommit

	_ = iter.ForEach(func(c *object.Commit) error {
		hash := c.Hash.String()[:8]
		author := c.Author.Name
		date := c.Author.When
		msg := strings.SplitN(c.Message, "\n", 2)[0]

		files := commitChangedFiles(c)
		for _, f := range files {
			dir := filepath.Dir(f)
			if dir == "." {
				continue
			}
			full := filepath.Join(absRoot, dir)
			rel, err := filepath.Rel(absRoot, full)
			if err != nil {
				continue
			}
			pkg := filepath.ToSlash(rel)

			commits = append(commits, PackageCommit{
				Package: pkg,
				Hash:    hash,
				Author:  author,
				Date:    date,
				Message: msg,
			})
		}
		return nil
	})

	seen := make(map[string]bool)
	deduped := make([]PackageCommit, 0, len(commits))
	for _, c := range commits {
		key := c.Hash + "|" + c.Package
		if seen[key] {
			continue
		}
		seen[key] = true
		deduped = append(deduped, c)
	}

	sort.Slice(deduped, func(i, j int) bool { return deduped[i].Date.After(deduped[j].Date) })
	return deduped
}

// AuthorOwnership returns per-package top contributors from git history.
func AuthorOwnership(root, modPath string) map[string][]Author {
	repo, err := gogit.PlainOpen(root)
	if err != nil {
		return nil
	}

	iter, err := repo.Log(&gogit.LogOptions{})
	if err != nil {
		return nil
	}

	absRoot, _ := filepath.Abs(root)
	pkgAuthors := make(map[string]map[string]int)

	_ = iter.ForEach(func(c *object.Commit) error {
		author := c.Author.Name
		files := commitChangedFiles(c)
		for _, f := range files {
			dir := filepath.Dir(f)
			if dir == "." {
				continue
			}
			full := filepath.Join(absRoot, dir)
			rel, err := filepath.Rel(absRoot, full)
			if err != nil {
				continue
			}
			pkg := filepath.ToSlash(rel)

			if pkgAuthors[pkg] == nil {
				pkgAuthors[pkg] = make(map[string]int)
			}
			pkgAuthors[pkg][author]++
		}
		return nil
	})

	result := make(map[string][]Author, len(pkgAuthors))
	for pkg, authors := range pkgAuthors {
		var list []Author
		for name, count := range authors {
			list = append(list, Author{Name: name, Commits: count})
		}
		sort.Slice(list, func(i, j int) bool { return list[i].Commits > list[j].Commits })
		if len(list) > MaxAuthorsPerPackage {
			list = list[:MaxAuthorsPerPackage]
		}
		result[pkg] = list
	}
	return result
}

// FileHotSpots returns the most-changed individual files from git history.
func FileHotSpots(root string, days int) []HotFile {
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
	fileCounts := make(map[string]int)

	_ = iter.ForEach(func(c *object.Commit) error {
		for _, f := range commitChangedFiles(c) {
			fileCounts[f]++
		}
		return nil
	})

	files := make([]HotFile, 0, len(fileCounts))
	for path, count := range fileCounts {
		dir := filepath.Dir(path)
		if dir == "." {
			dir = ""
		}
		full := filepath.Join(absRoot, dir)
		rel, err := filepath.Rel(absRoot, full)
		if err != nil {
			rel = dir
		}
		pkg := filepath.ToSlash(rel)
		files = append(files, HotFile{Path: path, Package: pkg, Changes: count})
	}

	sort.Slice(files, func(i, j int) bool { return files[i].Changes > files[j].Changes })
	if len(files) > MaxFileHotSpots {
		files = files[:MaxFileHotSpots]
	}
	return files
}
