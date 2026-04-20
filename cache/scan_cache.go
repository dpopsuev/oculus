package cache

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"

	oculus "github.com/dpopsuev/oculus/v3"
)

// ErrEmptySHA is returned when Put is called with an empty SHA.
var ErrEmptySHA = errors.New("empty SHA")

// ScanCache stores and retrieves architecture scan results on the filesystem,
// keyed by (repo path, git SHA, locus version). The version component ensures
// scanner bug fixes invalidate stale cache entries (BUG-30).
type ScanCache struct {
	root    string // e.g. ~/.locus/cache
	version string // locus build version for cache busting
}

// New creates a ScanCache. The version string is included in cache keys
// so scanner fixes invalidate stale entries.
func New(root string) *ScanCache {
	return &ScanCache{root: root, version: "dev"}
}

// NewVersioned creates a ScanCache with a specific version for cache busting.
func NewVersioned(root, version string) *ScanCache {
	return &ScanCache{root: root, version: version}
}

func (c *ScanCache) Root() string { return c.root }

// DefaultCacheDir returns ~/.locus/cache.
func DefaultCacheDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".locus", "cache")
}

// Get returns a cached report for the given repo at the given SHA.
func (c *ScanCache) Get(repoPath, sha string) (*oculus.ContextReport, bool, error) {
	if sha == "" {
		return nil, false, nil
	}
	p := c.entryPath(repoPath, sha)
	f, err := os.Open(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, false, nil
	}
	defer gz.Close()

	var report oculus.ContextReport
	if err := json.NewDecoder(gz).Decode(&report); err != nil {
		return nil, false, nil
	}
	return &report, true, nil
}

// GetCurrent resolves HEAD for the repo and returns the cached report if present.
// Returns the resolved SHA alongside the report.
func (c *ScanCache) GetCurrent(repoPath string) (report *oculus.ContextReport, sha string, hit bool, err error) {
	sha = ResolveHEAD(repoPath)
	if sha == "" {
		return nil, "", false, nil
	}
	report, hit, err = c.Get(repoPath, sha)
	return report, sha, hit, err
}

// Put stores a report keyed by (repo, sha). Writes are atomic (temp + rename).
func (c *ScanCache) Put(repoPath, sha string, report *oculus.ContextReport) error {
	if sha == "" {
		return ErrEmptySHA
	}
	p := c.entryPath(repoPath, sha)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(p), ".tmp-*")
	if err != nil {
		return err
	}
	defer func() {
		tmp.Close()
		os.Remove(tmp.Name())
	}()

	gz := gzip.NewWriter(tmp)
	if err := json.NewEncoder(gz).Encode(report); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), p)
}

// GetGraph returns a cached SymbolGraph for the given repo at the given SHA.
func (c *ScanCache) GetGraph(repoPath, sha string) (*oculus.SymbolGraph, bool, error) {
	if sha == "" {
		return nil, false, nil
	}
	p := c.graphPath(repoPath, sha)
	f, err := os.Open(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, false, nil
	}
	defer gz.Close()

	var sg oculus.SymbolGraph
	if err := json.NewDecoder(gz).Decode(&sg); err != nil {
		return nil, false, nil
	}
	return &sg, true, nil
}

// PutGraph stores a SymbolGraph keyed by (repo, sha). Atomic write.
func (c *ScanCache) PutGraph(repoPath, sha string, sg *oculus.SymbolGraph) error {
	if sha == "" {
		return ErrEmptySHA
	}
	p := c.graphPath(repoPath, sha)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(p), ".tmp-*")
	if err != nil {
		return err
	}
	defer func() {
		tmp.Close()
		os.Remove(tmp.Name())
	}()

	gz := gzip.NewWriter(tmp)
	if err := json.NewEncoder(gz).Encode(sg); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), p)
}

func (c *ScanCache) graphPath(repoPath, sha string) string {
	vHash := fmt.Sprintf("%x", sha256.Sum256([]byte(c.version)))[:8]
	return filepath.Join(c.root, RepoHash(repoPath), sha+"-"+vHash+".graph.json.gz")
}

// Invalidate removes all cached entries for a repo.
func (c *ScanCache) Invalidate(repoPath string) error {
	dir := filepath.Join(c.root, RepoHash(repoPath))
	return os.RemoveAll(dir)
}

func (c *ScanCache) entryPath(repoPath, sha string) string {
	// BUG-30: include version in filename so scanner fixes bust the cache.
	vHash := fmt.Sprintf("%x", sha256.Sum256([]byte(c.version)))[:8]
	return filepath.Join(c.root, RepoHash(repoPath), sha+"-"+vHash+".json.gz")
}

// RepoHash returns a deterministic hash for a repository path.
func RepoHash(repoPath string) string {
	abs, err := filepath.Abs(repoPath)
	if err != nil {
		abs = repoPath
	}
	h := sha256.Sum256([]byte(abs))
	return fmt.Sprintf("%x", h[:8])
}

// ResolveHEAD returns the current HEAD SHA for a git repo, or "" if not a repo.
func ResolveHEAD(repoPath string) string {
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return ""
	}
	ref, err := repo.Head()
	if err != nil {
		return ""
	}
	return ref.Hash().String()
}

// ResolveBranch returns the SHA for a named branch/ref in a git repo.
func ResolveBranch(repoPath, refName string) (string, error) {
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return "", fmt.Errorf("open repo: %w", err)
	}
	h, err := repo.ResolveRevision(plumbing.Revision(refName))
	if err != nil {
		return "", fmt.Errorf("resolve %q: %w", refName, err)
	}
	return h.String(), nil
}
