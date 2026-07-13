package engine

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/dpopsuev/oculus/v3/locator"
	"github.com/dpopsuev/oculus/v3/lsp"
)

// RenameOpts controls dry-run vs apply for GetRename.
type RenameOpts struct {
	// Apply writes the WorkspaceEdit to disk. Default false (dry-run).
	Apply bool
}

// RenameReport is the result of prepareRename + rename with a refs coverage gate.
type RenameReport struct {
	Locator       string             `json:"locator"`
	Symbol        string             `json:"symbol,omitempty"`
	NewName       string             `json:"new_name"`
	DryRun        bool               `json:"dry_run"`
	Prepared      bool               `json:"prepared"`
	Placeholder   string             `json:"placeholder,omitempty"`
	RefCount      int                `json:"ref_count"`
	RefFiles      int                `json:"ref_files"`
	EditCount     int                `json:"edit_count"`
	EditFiles     int                `json:"edit_files"`
	CoverageOK    bool               `json:"coverage_ok"`
	CoverageNote  string             `json:"coverage_note,omitempty"`
	Applied       bool               `json:"applied"`
	Rebound       bool               `json:"rebound"`
	Edit          *lsp.WorkspaceEdit `json:"edit,omitempty"`
	Escalations   []string           `json:"escalations,omitempty"`
	Candidates    []locator.Hit      `json:"candidates,omitempty"`
	Summary       string             `json:"summary"`
}

// GetRename resolves locator, runs prepareRename, probes references, requests
// rename, and gates on refs coverage. Dry-run by default; Apply writes edits
// then MarkDirty (Merkle dirty flag + SG flush).
func (p *Engine) GetRename(ctx context.Context, path, raw, newName string, opts RenameOpts, cacheKey ...string) (*RenameReport, error) {
	if newName == "" {
		return nil, fmt.Errorf("rename requires new_name")
	}
	hit, unresolved, err := p.resolveUniqueHit(ctx, path, raw, cacheKey...)
	if err != nil {
		return nil, err
	}
	if unresolved != nil {
		return &RenameReport{
			Locator:     raw,
			NewName:     newName,
			DryRun:      !opts.Apply,
			Escalations: unresolved.Escalations,
			Candidates:  unresolved.Candidates,
			Summary:     unresolved.Summary,
		}, nil
	}

	file, line, char, err := absHit(path, hit)
	if err != nil {
		return nil, err
	}
	rep := &RenameReport{
		Locator: raw,
		Symbol:  hit.FQN,
		NewName: newName,
		DryRun:  !opts.Apply,
	}

	prep, err := p.lspPrepareRename(ctx, file, line, char)
	if err != nil {
		rep.Summary = "rename unavailable: " + err.Error()
		return rep, nil
	}
	if prep == nil || !prep.OK {
		rep.Summary = fmt.Sprintf("prepareRename rejected rename of %s", hit.FQN)
		return rep, nil
	}
	rep.Prepared = true
	rep.Placeholder = prep.Placeholder

	refs, err := p.lspReferences(ctx, file, line, char)
	if err != nil {
		// Soft: continue with zero refs; coverage may still pass for single-site.
		refs = nil
	}
	rep.RefCount = len(refs)
	rep.RefFiles = distinctLocFiles(refs)

	edit, err := p.lspRename(ctx, file, line, char, newName)
	if err != nil {
		rep.Summary = "rename unavailable: " + err.Error()
		return rep, nil
	}
	rep.Edit = edit
	rep.EditCount = edit.EditCount()
	rep.EditFiles = edit.FileCount()
	rep.CoverageOK, rep.CoverageNote = renameCoverageOK(rep.RefCount, rep.RefFiles, rep.EditCount, rep.EditFiles)

	if !rep.CoverageOK {
		rep.Summary = fmt.Sprintf("rename coverage gate failed for %s→%s: %s", hit.FQN, newName, rep.CoverageNote)
		return rep, nil
	}

	if !opts.Apply {
		rep.Summary = fmt.Sprintf("dry-run rename %s→%s: %d edit(s) in %d file(s) (refs=%d)", hit.FQN, newName, rep.EditCount, rep.EditFiles, rep.RefCount)
		return rep, nil
	}

	if err := lsp.ApplyWorkspaceEdit(edit); err != nil {
		rep.Summary = "apply failed: " + err.Error()
		return rep, nil
	}
	rep.Applied = true
	p.RebindAfterMutation(path)
	rep.Rebound = true
	rep.DryRun = false
	rep.Summary = fmt.Sprintf("applied rename %s→%s: %d edit(s) in %d file(s); graph rebound", hit.FQN, newName, rep.EditCount, rep.EditFiles)
	return rep, nil
}

// RebindAfterMutation marks the workspace dirty and flushes the symbol-graph
// cache so the next scan/analysis sees Merkle-fresh state after disk mutation.
func (p *Engine) RebindAfterMutation(path string) {
	p.MarkDirty(path)
}

func renameCoverageOK(refCount, refFiles, editCount, editFiles int) (bool, string) {
	if editCount == 0 {
		return false, "WorkspaceEdit empty"
	}
	if refFiles > 1 && editFiles <= 1 {
		return false, fmt.Sprintf("expected multi-file edit (refs in %d files), got %d file(s)", refFiles, editFiles)
	}
	if refCount > 1 && editCount < refCount {
		return false, fmt.Sprintf("edit sites %d < reference sites %d", editCount, refCount)
	}
	return true, fmt.Sprintf("edits=%d files=%d cover refs=%d files=%d", editCount, editFiles, refCount, refFiles)
}

func distinctLocFiles(locs []lsp.Location) int {
	seen := map[string]bool{}
	for _, l := range locs {
		file := filepath.Clean(trimFileURI(l.URI))
		seen[file] = true
	}
	return len(seen)
}

func trimFileURI(uri string) string {
	const p = "file://"
	if len(uri) >= len(p) && uri[:len(p)] == p {
		return uri[len(p):]
	}
	return uri
}

func (p *Engine) lspPrepareRename(ctx context.Context, file string, line, char int) (*lsp.PrepareResult, error) {
	if p.pool == nil {
		return nil, lsp.ErrNoPool
	}
	return p.pool.PrepareRename(ctx, file, line, char)
}

func (p *Engine) lspRename(ctx context.Context, file string, line, char int, newName string) (*lsp.WorkspaceEdit, error) {
	if p.pool == nil {
		return nil, lsp.ErrNoPool
	}
	return p.pool.Rename(ctx, file, line, char, newName)
}
