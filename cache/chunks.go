package cache

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

// Chunk is one retrieval unit (usually a symbol body excerpt).
type Chunk struct {
	ID     string `json:"id"`
	Symbol string `json:"symbol,omitempty"`
	Kind   string `json:"kind,omitempty"`
	File   string `json:"file"`
	Text   string `json:"text"`
	Start  int    `json:"start,omitempty"`
	End    int    `json:"end,omitempty"`
}

// ChunkHit is a ranked chunk from BM25-lite search.
type ChunkHit struct {
	Chunk
	Score float64 `json:"score"`
}

// ChunkIndex stores chunks for one repo merkle snapshot.
type ChunkIndex struct {
	RepoHash   string  `json:"repo_hash"`
	MerkleRoot string  `json:"merkle_root"`
	Chunks     []Chunk `json:"chunks"`
}

// BuildChunksFromFiles walks source files and creates line-window chunks plus
// simple symbol heuristics (func/type lines in Go).
func BuildChunksFromFiles(root string, maxPerFile int) ([]Chunk, error) {
	if maxPerFile <= 0 {
		maxPerFile = 40
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	idx, err := BuildMerkle(abs)
	if err != nil {
		return nil, err
	}
	var out []Chunk
	for rel := range idx.Leaves {
		if !strings.HasSuffix(rel, ".go") && !strings.HasSuffix(rel, ".ts") &&
			!strings.HasSuffix(rel, ".py") && !strings.HasSuffix(rel, ".rs") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(abs, filepath.FromSlash(rel)))
		if err != nil {
			continue
		}
		chunks := chunkFile(rel, string(data), maxPerFile)
		out = append(out, chunks...)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func chunkFile(rel, body string, maxPerFile int) []Chunk {
	lines := strings.Split(body, "\n")
	var out []Chunk
	// Symbol-oriented: capture func/type declarations with following block (~20 lines).
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		var sym, kind string
		switch {
		case strings.HasPrefix(trim, "func "):
			kind = "function"
			sym = extractGoFuncName(trim)
		case strings.HasPrefix(trim, "type ") && strings.Contains(trim, "struct"):
			kind = "type"
			sym = extractGoTypeName(trim)
		case strings.HasPrefix(trim, "def "):
			kind = "function"
			sym = strings.TrimSuffix(strings.TrimPrefix(trim, "def "), ":")
			sym = strings.Fields(sym)[0]
		default:
			continue
		}
		if sym == "" {
			continue
		}
		end := i + 20
		if end > len(lines) {
			end = len(lines)
		}
		text := strings.Join(lines[i:end], "\n")
		id := rel + "#" + sym + "#" + itoa(i+1)
		out = append(out, Chunk{
			ID: id, Symbol: sym, Kind: kind, File: rel,
			Text: text, Start: i + 1, End: end,
		})
		if len(out) >= maxPerFile {
			return out
		}
	}
	// Fallback: one window for the whole file if no symbols found.
	if len(out) == 0 && strings.TrimSpace(body) != "" {
		excerpt := body
		if len(excerpt) > 2000 {
			excerpt = excerpt[:2000]
		}
		out = append(out, Chunk{
			ID: rel + "#file", File: rel, Text: excerpt, Kind: "file",
			Symbol: filepath.Base(rel),
		})
	}
	return out
}

func extractGoFuncName(line string) string {
	// func (r *T) Name( or func Name(
	line = strings.TrimPrefix(line, "func ")
	if strings.HasPrefix(line, "(") {
		if idx := strings.Index(line, ")"); idx >= 0 {
			line = strings.TrimSpace(line[idx+1:])
		}
	}
	name := strings.FieldsFunc(line, func(r rune) bool {
		return r == '(' || unicode.IsSpace(r)
	})
	if len(name) == 0 {
		return ""
	}
	return name[0]
}

func extractGoTypeName(line string) string {
	line = strings.TrimPrefix(strings.TrimSpace(line), "type ")
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [16]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// SearchChunks ranks chunks with a BM25-lite score over query terms.
func SearchChunks(chunks []Chunk, query string, limit int) []ChunkHit {
	terms := tokenize(query)
	if len(terms) == 0 || limit <= 0 {
		return nil
	}
	N := float64(len(chunks))
	df := map[string]int{}
	for _, t := range terms {
		for _, c := range chunks {
			if strings.Contains(strings.ToLower(c.Text+" "+c.Symbol+" "+c.File), t) {
				df[t]++
			}
		}
	}
	var hits []ChunkHit
	for _, c := range chunks {
		blob := strings.ToLower(c.Text + " " + c.Symbol + " " + c.File)
		var score float64
		for _, t := range terms {
			if !strings.Contains(blob, t) {
				continue
			}
			// idf-ish
			idf := 1.0
			if df[t] > 0 {
				idf = (N - float64(df[t]) + 0.5) / (float64(df[t]) + 0.5)
				if idf < 0.1 {
					idf = 0.1
				}
			}
			tf := float64(strings.Count(blob, t))
			score += idf * tf
			if strings.Contains(strings.ToLower(c.Symbol), t) {
				score += 5
			}
		}
		if score > 0 {
			hits = append(hits, ChunkHit{Chunk: c, Score: score})
		}
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		return hits[i].ID < hits[j].ID
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}

func tokenize(q string) []string {
	fields := strings.FieldsFunc(strings.ToLower(q), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	stop := map[string]bool{
		"where": true, "is": true, "the": true, "a": true, "an": true,
		"for": true, "to": true, "of": true, "in": true, "and": true,
		"how": true, "what": true, "does": true, "do": true,
	}
	var out []string
	seen := map[string]bool{}
	for _, f := range fields {
		if len(f) < 3 || stop[f] || seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, f)
	}
	return out
}
