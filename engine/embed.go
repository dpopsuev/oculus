package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strings"
)

// Embedder produces vector embeddings for texts.
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

// HTTPEmbedder calls an OpenAI-compatible /v1/embeddings endpoint.
type HTTPEmbedder struct {
	URL    string
	APIKey string
	Model  string
	Client *http.Client
}

type embedReq struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type embedResp struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
}

func (e *HTTPEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if e.URL == "" {
		return nil, fmt.Errorf("empty embed URL")
	}
	model := e.Model
	if model == "" {
		model = "nomic-embed-text"
	}
	body, _ := json.Marshal(embedReq{Model: model, Input: texts})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.URL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if e.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+e.APIKey)
	}
	client := e.Client
	if client == nil {
		client = http.DefaultClient
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		return nil, fmt.Errorf("embed HTTP %d: %s", res.StatusCode, truncate(string(raw), 200))
	}
	var parsed embedResp
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, err
	}
	out := make([][]float32, len(texts))
	for _, d := range parsed.Data {
		if d.Index >= 0 && d.Index < len(out) {
			out[d.Index] = d.Embedding
		}
	}
	return out, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// NewEmbedderFromEnv returns an HTTPEmbedder when LOCUS_EMBED_URL is set.
func NewEmbedderFromEnv() Embedder {
	url := strings.TrimSpace(os.Getenv("LOCUS_EMBED_URL"))
	if url == "" {
		return nil
	}
	return &HTTPEmbedder{
		URL:    url,
		APIKey: os.Getenv("LOCUS_EMBED_API_KEY"),
		Model:  os.Getenv("LOCUS_EMBED_MODEL"),
	}
}

// Cosine similarity; returns 0 for zero vectors.
func cosine(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// RRF merges ranked ID lists with reciprocal rank fusion.
func RRF(lists ...[]string) []string {
	const k = 60.0
	scores := map[string]float64{}
	for _, list := range lists {
		for i, id := range list {
			scores[id] += 1.0 / (k + float64(i+1))
		}
	}
	type pair struct {
		id string
		s  float64
	}
	var pairs []pair
	for id, s := range scores {
		pairs = append(pairs, pair{id, s})
	}
	for i := 0; i < len(pairs); i++ {
		for j := i + 1; j < len(pairs); j++ {
			if pairs[j].s > pairs[i].s || (pairs[j].s == pairs[i].s && pairs[j].id < pairs[i].id) {
				pairs[i], pairs[j] = pairs[j], pairs[i]
			}
		}
	}
	out := make([]string, len(pairs))
	for i, p := range pairs {
		out[i] = p.id
	}
	return out
}
