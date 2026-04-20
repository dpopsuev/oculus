package remote

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"

	"github.com/dpopsuev/oculus/v3/arch"
)

type Opts struct {
	Ref       string
	Keep      bool
	Depth     int
	ChurnDays int
	Budget    int
	Intent    string
}

type Result struct {
	Report   *arch.ContextReport
	CloneDir string
	RefSHA   string
}

func CloneDir(repoURL, ref string) string {
	home, _ := os.UserHomeDir()
	raw := repoURL + "\n" + ref
	h := sha256.Sum256([]byte(raw))
	return filepath.Join(home, ".locus", "clones", fmt.Sprintf("%x", h[:8]))
}

func ScanRemote(ctx context.Context, repoURL string, opts Opts) (*Result, error) {
	ref := opts.Ref
	if ref == "" {
		ref = "HEAD"
	}

	dir := CloneDir(repoURL, ref)

	if err := shallowClone(ctx, repoURL, ref, dir); err != nil {
		return nil, fmt.Errorf("shallow clone %s: %w", repoURL, err)
	}

	if !opts.Keep {
		defer os.RemoveAll(dir)
	}

	churnDays := opts.ChurnDays
	if churnDays == 0 {
		churnDays = 30
	}

	report, err := arch.ScanAndBuild(ctx, dir, arch.ScanOpts{
		ExcludeTests: true,
		Depth:        opts.Depth,
		ChurnDays:    churnDays,
		Budget:       opts.Budget,
		Intent:       arch.ScanIntent(opts.Intent),
	})
	if err != nil {
		return nil, fmt.Errorf("scan %s: %w", repoURL, err)
	}

	sha := resolveHEAD(dir)

	return &Result{
		Report:   report,
		CloneDir: dir,
		RefSHA:   sha,
	}, nil
}

func shallowClone(ctx context.Context, repoURL, ref, dir string) error {
	normalizedURL := NormalizeURL(repoURL)

	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		// Existing clone — fetch the ref and check it out.
		repo, err := gogit.PlainOpen(dir)
		if err != nil {
			return fmt.Errorf("git fetch: open repo: %w", err)
		}

		refSpec := config.RefSpec(fmt.Sprintf("+%s:refs/remotes/origin/FETCH_HEAD", ref))
		if plumbing.IsHash(ref) || ref == "HEAD" {
			refSpec = config.RefSpec(fmt.Sprintf("+refs/*:refs/remotes/origin/*"))
		}

		if err := repo.FetchContext(ctx, &gogit.FetchOptions{
			Depth:    1,
			RefSpecs: []config.RefSpec{refSpec},
		}); err != nil && err != gogit.NoErrAlreadyUpToDate {
			return fmt.Errorf("git fetch: %w", err)
		}

		// Resolve the ref to a hash, then force-checkout.
		h, err := repo.ResolveRevision(plumbing.Revision(ref))
		if err != nil {
			return fmt.Errorf("git fetch: resolve %q: %w", ref, err)
		}
		wt, err := repo.Worktree()
		if err != nil {
			return fmt.Errorf("git reset: worktree: %w", err)
		}
		if err := wt.Checkout(&gogit.CheckoutOptions{Hash: *h, Force: true}); err != nil {
			return fmt.Errorf("git reset: %w", err)
		}
		return nil
	}

	// Fresh clone.
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return err
	}

	cloneOpts := &gogit.CloneOptions{
		URL:   normalizedURL,
		Depth: 1,
	}
	if ref != "HEAD" {
		cloneOpts.ReferenceName = plumbing.NewBranchReferenceName(ref)
		cloneOpts.SingleBranch = true
	}

	if _, err := gogit.PlainCloneContext(ctx, dir, false, cloneOpts); err != nil {
		return fmt.Errorf("git clone: %w", err)
	}
	return nil
}

// NormalizeURL converts shorthand GitHub URLs to full HTTPS URLs.
// Handles: github.com/org/repo, https://github.com/org/repo, git@github.com:org/repo.git
func NormalizeURL(raw string) string {
	if strings.HasPrefix(raw, "git@") {
		raw = strings.TrimPrefix(raw, "git@")
		raw = strings.Replace(raw, ":", "/", 1)
		raw = strings.TrimSuffix(raw, ".git")
		return "https://" + raw
	}

	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}

	if u, err := url.Parse(raw); err == nil {
		u.Path = strings.TrimSuffix(u.Path, ".git")
		return u.String()
	}

	return raw
}

// CacheKey returns a deterministic key for caching remote codographs.
// Normalizes the URL so that shorthand (github.com/foo/bar) and full
// (https://github.com/foo/bar) produce the same key. BUG-11.
func CacheKey(repoURL, refSHA string) string {
	return "remote:" + NormalizeURL(repoURL) + "@" + refSHA
}

func resolveHEAD(dir string) string {
	repo, err := gogit.PlainOpen(dir)
	if err != nil {
		return ""
	}
	ref, err := repo.Head()
	if err != nil {
		return ""
	}
	return ref.Hash().String()
}
