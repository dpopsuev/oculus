package triage

import (
	"strings"
	"unicode"
)

// tokenize splits an intent string into lowercase word tokens, stripping
// punctuation and common stop words.
func tokenize(intent string) []string {
	words := strings.FieldsFunc(strings.ToLower(intent), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	var tokens []string
	for _, w := range words {
		if !stopWord(w) {
			tokens = append(tokens, w)
		}
	}
	return tokens
}

// intentCoverageScore returns how much of the intent is covered by tool
// keywords: |matched intent tokens| / |intent tokens|.
//
// Unlike Jaccard, this does not punish tools with large keyword lists
// (fat MCP tools like "analysis"), which previously crushed confidence
// to ~0.02 for clear intents such as "find coupling hotspots".
func intentCoverageScore(intentTokens, keywords []string) float64 {
	if len(intentTokens) == 0 || len(keywords) == 0 {
		return 0
	}

	kwSet := make(map[string]bool, len(keywords))
	for _, k := range keywords {
		kwSet[strings.ToLower(k)] = true
	}

	matched := 0
	intentSet := make(map[string]bool, len(intentTokens))
	for _, t := range intentTokens {
		if intentSet[t] {
			continue
		}
		intentSet[t] = true
		if keywordMatches(t, kwSet) {
			matched++
		}
	}
	if matched == 0 {
		return 0
	}
	return float64(matched) / float64(len(intentSet))
}

// jaccardScore is retained for tests/compat; scoring uses intentCoverageScore.
func jaccardScore(intentTokens, keywords []string) float64 {
	return intentCoverageScore(intentTokens, keywords)
}

func keywordMatches(token string, kwSet map[string]bool) bool {
	if kwSet[token] {
		return true
	}
	// Prefix matching: "perform" ↔ "performance", "hotspot" ↔ "hot"
	for kw := range kwSet {
		if strings.HasPrefix(kw, token) || strings.HasPrefix(token, kw) {
			return true
		}
	}
	return false
}

var stops = map[string]bool{
	"a": true, "an": true, "the": true, "is": true, "are": true,
	"in": true, "on": true, "at": true, "to": true, "for": true,
	"of": true, "and": true, "or": true, "my": true, "me": true,
	"i": true, "it": true, "do": true, "does": true, "this": true,
	"that": true, "with": true, "from": true, "can": true, "how": true,
	"find": true, "show": true, "get": true, "please": true, "about": true,
}

func stopWord(w string) bool {
	return stops[w]
}
