// Package context provides scoped project context storage for AI agents.
// Context entries are stored on disk and keyed by repository, scope, and target.
package context

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/dpopsuev/oculus/v3/cache"
)

// Scope identifies the granularity of a context entry.
type Scope string

const (
	ScopeProject Scope = "project"
	ScopeModule  Scope = "module"
	ScopeFile    Scope = "file"
	ScopeSymbol  Scope = "symbol"
)

// ContextEntry is a stored context record with staleness tracking.
type ContextEntry struct {
	Content    string `json:"content"`
	SHA        string `json:"sha"`
	CurrentSHA string `json:"current_sha"`
	Stale      bool   `json:"stale"`
	Scope      Scope  `json:"scope"`
	Target     string `json:"target"`
}

// Store manages context entries on disk.
type Store struct {
	root string // e.g. $XDG_DATA_HOME/oculus/context
}

// DefaultDir returns $XDG_DATA_HOME/oculus/context
// (falls back to ~/.local/share/oculus/context).
func DefaultDir() string {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "oculus", "context")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "oculus", "context")
}

// New creates a Store rooted at the given directory.
func New(root string) *Store {
	return &Store{root: root}
}

// Read returns the context entry for the given repo, scope, and target.
// Returns nil, nil if no entry exists.
func (s *Store) Read(repoPath string, scope Scope, target string) (*ContextEntry, error) {
	p := s.entryPath(repoPath, scope, target)
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	// Read the SHA from the companion .sha file.
	sha, _ := os.ReadFile(p + ".sha")

	currentSHA := cache.ResolveHEAD(repoPath)
	stale := string(sha) != "" && currentSHA != "" && string(sha) != currentSHA

	return &ContextEntry{
		Content:    string(data),
		SHA:        string(sha),
		CurrentSHA: currentSHA,
		Stale:      stale,
		Scope:      scope,
		Target:     target,
	}, nil
}

// Write stores a context entry for the given repo, scope, and target.
func (s *Store) Write(repoPath string, scope Scope, target, sha, content string) error {
	p := s.entryPath(repoPath, scope, target)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		return err
	}
	return os.WriteFile(p+".sha", []byte(sha), 0o644)
}

// entryPath returns the filesystem path for a context entry:
// {root}/{RepoHash(repoPath)}/{scope}/{sanitized-target}.md
func (s *Store) entryPath(repoPath string, scope Scope, target string) string {
	return filepath.Join(
		s.root,
		cache.RepoHash(repoPath),
		string(scope),
		sanitize(target)+".md",
	)
}

// sanitize replaces path separators and other problematic characters with dashes.
func sanitize(target string) string {
	r := strings.NewReplacer(
		"/", "-",
		"\\", "-",
		":", "-",
		" ", "_",
		"..", "_",
	)
	return r.Replace(target)
}
