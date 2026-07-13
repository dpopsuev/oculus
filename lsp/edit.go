package lsp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

// PrepareResult is the outcome of textDocument/prepareRename.
type PrepareResult struct {
	OK          bool   `json:"ok"`
	Placeholder string `json:"placeholder,omitempty"`
	StartLine   int    `json:"start_line,omitempty"` // 1-based
	StartChar   int    `json:"start_char,omitempty"`
	EndLine     int    `json:"end_line,omitempty"`
	EndChar     int    `json:"end_char,omitempty"`
	Default     bool   `json:"default_behavior,omitempty"`
}

// TextEdit is a single LSP text edit. Lines/chars are 0-based (LSP).
type TextEdit struct {
	StartLine int    `json:"start_line"`
	StartChar int    `json:"start_char"`
	EndLine   int    `json:"end_line"`
	EndChar   int    `json:"end_char"`
	NewText   string `json:"new_text"`
}

// FileEdit groups text edits for one file.
type FileEdit struct {
	File  string     `json:"file"`
	Edits []TextEdit `json:"edits"`
}

// WorkspaceEdit is a compact rename plan (changes + documentChanges flattened).
type WorkspaceEdit struct {
	Files []FileEdit `json:"files"`
}

// EditCount returns total text edits across files.
func (e *WorkspaceEdit) EditCount() int {
	if e == nil {
		return 0
	}
	n := 0
	for _, f := range e.Files {
		n += len(f.Edits)
	}
	return n
}

// FileCount returns distinct files touched.
func (e *WorkspaceEdit) FileCount() int {
	if e == nil {
		return 0
	}
	return len(e.Files)
}

// poolPrepareRename validates rename at position.
func poolPrepareRename(ctx context.Context, pool Pool, file string, line, char int) (*PrepareResult, error) {
	language := fileLanguage(file)
	root := workspaceRootForFile(file)
	client, err := pool.Get(language, root)
	if err != nil {
		return nil, err
	}
	defer pool.Release(language, root)

	uri := openFile(client, file)
	raw, err := client.RequestContext(ctx, "textDocument/prepareRename", map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"position":     map[string]any{"line": line - 1, "character": char},
	})
	if err != nil {
		return &PrepareResult{OK: false}, nil
	}
	return parsePrepareRename(raw), nil
}

func parsePrepareRename(raw json.RawMessage) *PrepareResult {
	if len(raw) == 0 || string(raw) == "null" {
		return &PrepareResult{OK: false}
	}
	var def struct {
		DefaultBehavior bool `json:"defaultBehavior"`
	}
	if err := json.Unmarshal(raw, &def); err == nil && def.DefaultBehavior {
		return &PrepareResult{OK: true, Default: true}
	}
	type lspPos struct {
		Line      int `json:"line"`
		Character int `json:"character"`
	}
	type lspRange struct {
		Start lspPos `json:"start"`
		End   lspPos `json:"end"`
	}
	var withPH struct {
		Range       lspRange `json:"range"`
		Placeholder string   `json:"placeholder"`
	}
	if err := json.Unmarshal(raw, &withPH); err == nil && (withPH.Range.End.Line > withPH.Range.Start.Line || withPH.Range.End.Character >= withPH.Range.Start.Character) {
		if withPH.Placeholder != "" || withPH.Range != (lspRange{}) {
			return &PrepareResult{
				OK:          true,
				Placeholder: withPH.Placeholder,
				StartLine:   withPH.Range.Start.Line + 1,
				StartChar:   withPH.Range.Start.Character,
				EndLine:     withPH.Range.End.Line + 1,
				EndChar:     withPH.Range.End.Character,
			}
		}
	}
	var justRange lspRange
	if err := json.Unmarshal(raw, &justRange); err == nil {
		return &PrepareResult{
			OK:        true,
			StartLine: justRange.Start.Line + 1,
			StartChar: justRange.Start.Character,
			EndLine:   justRange.End.Line + 1,
			EndChar:   justRange.End.Character,
		}
	}
	return &PrepareResult{OK: false}
}

// poolRename issues textDocument/rename and parses WorkspaceEdit.
func poolRename(ctx context.Context, pool Pool, file string, line, char int, newName string) (*WorkspaceEdit, error) {
	language := fileLanguage(file)
	root := workspaceRootForFile(file)
	client, err := pool.Get(language, root)
	if err != nil {
		return nil, err
	}
	defer pool.Release(language, root)

	uri := openFile(client, file)
	raw, err := client.RequestContext(ctx, "textDocument/rename", map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"position":     map[string]any{"line": line - 1, "character": char},
		"newName":      newName,
	})
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 || string(raw) == "null" {
		return &WorkspaceEdit{}, nil
	}
	return parseWorkspaceEdit(raw), nil
}

func openFile(client *Client, file string) string {
	content, _ := os.ReadFile(file)
	uri := "file://" + filepath.ToSlash(file)
	langID := extToLangID(filepath.Ext(file))
	_ = client.Notify("textDocument/didOpen", map[string]any{
		"textDocument": map[string]any{
			"uri":        uri,
			"languageId": langID,
			"version":    1,
			"text":       string(content),
		},
	})
	return uri
}

func parseWorkspaceEdit(raw json.RawMessage) *WorkspaceEdit {
	type lspPos struct {
		Line      int `json:"line"`
		Character int `json:"character"`
	}
	type lspRange struct {
		Start lspPos `json:"start"`
		End   lspPos `json:"end"`
	}
	type lspEdit struct {
		Range   lspRange `json:"range"`
		NewText string   `json:"newText"`
	}
	byFile := map[string][]TextEdit{}

	add := func(uri string, edits []lspEdit) {
		file := strings.TrimPrefix(uri, "file://")
		for _, e := range edits {
			byFile[file] = append(byFile[file], TextEdit{
				StartLine: e.Range.Start.Line,
				StartChar: e.Range.Start.Character,
				EndLine:   e.Range.End.Line,
				EndChar:   e.Range.End.Character,
				NewText:   e.NewText,
			})
		}
	}

	var we struct {
		Changes         map[string][]lspEdit `json:"changes"`
		DocumentChanges []json.RawMessage    `json:"documentChanges"`
	}
	if err := json.Unmarshal(raw, &we); err != nil {
		return &WorkspaceEdit{}
	}
	for uri, edits := range we.Changes {
		add(uri, edits)
	}
	for _, rawDC := range we.DocumentChanges {
		var te struct {
			TextDocument struct {
				URI string `json:"uri"`
			} `json:"textDocument"`
			Edits []lspEdit `json:"edits"`
		}
		if json.Unmarshal(rawDC, &te) == nil && te.TextDocument.URI != "" {
			add(te.TextDocument.URI, te.Edits)
			continue
		}
		// Annotated text edits: edits may be {textEdit:{...}} — skip complex ops for MVP.
	}

	files := make([]FileEdit, 0, len(byFile))
	for file, edits := range byFile {
		files = append(files, FileEdit{File: file, Edits: edits})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].File < files[j].File })
	return &WorkspaceEdit{Files: files}
}

// ApplyWorkspaceEdit writes text edits to disk. Edits within a file are
// applied bottom-up so earlier offsets stay valid.
func ApplyWorkspaceEdit(edit *WorkspaceEdit) error {
	if edit == nil {
		return nil
	}
	for _, fe := range edit.Files {
		if err := applyFileEdits(fe.File, fe.Edits); err != nil {
			return err
		}
	}
	return nil
}

func applyFileEdits(file string, edits []TextEdit) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	content := string(data)
	sorted := append([]TextEdit(nil), edits...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].StartLine != sorted[j].StartLine {
			return sorted[i].StartLine > sorted[j].StartLine
		}
		return sorted[i].StartChar > sorted[j].StartChar
	})
	for _, e := range sorted {
		start, err := offsetAt(content, e.StartLine, e.StartChar)
		if err != nil {
			return fmt.Errorf("%s: %w", file, err)
		}
		end, err := offsetAt(content, e.EndLine, e.EndChar)
		if err != nil {
			return fmt.Errorf("%s: %w", file, err)
		}
		if end < start {
			return fmt.Errorf("%s: invalid edit range", file)
		}
		content = content[:start] + e.NewText + content[end:]
	}
	return os.WriteFile(file, []byte(content), 0o644)
}

// offsetAt maps 0-based line/char to a byte offset (UTF-8 code points ≈ chars for ASCII).
func offsetAt(content string, line, char int) (int, error) {
	if line < 0 || char < 0 {
		return 0, fmt.Errorf("negative position")
	}
	curLine := 0
	for i := 0; i < len(content); {
		if curLine == line {
			// Count characters (runes) on this line.
			col := 0
			for i < len(content) && content[i] != '\n' {
				if col == char {
					return i, nil
				}
				_, size := utf8.DecodeRuneInString(content[i:])
				i += size
				col++
			}
			if col == char {
				return i, nil
			}
			return 0, fmt.Errorf("char %d past end of line %d", char, line)
		}
		if content[i] == '\n' {
			curLine++
			i++
			continue
		}
		_, size := utf8.DecodeRuneInString(content[i:])
		i += size
	}
	if curLine == line && char == 0 {
		return len(content), nil
	}
	return 0, fmt.Errorf("line %d past end of file", line)
}
