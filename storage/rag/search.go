package rag

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
)

// SourceSpan uses Unicode code-point offsets [start,end) in the saved document.
type SourceSpan struct {
	DocumentSHA256 string `json:"document_sha256"`
	BlockID        string `json:"block_id"`
	Start          int    `json:"start"`
	End            int    `json:"end"`
	Quote          string `json:"quote"`
}

type Situation struct {
	Schema      string       `json:"schema"`
	ID          string       `json:"situation_id"`
	AnswerText  string       `json:"answer_text"`
	SourceSpans []SourceSpan `json:"source_spans"`
}

// ApplySituation keeps retrieval text separate from the generator's evidence.
func ApplySituation(r *SearchResult, metadata []byte) error {
	var m struct {
		Situation *Situation `json:"situation,omitempty"`
	}
	if err := json.Unmarshal(metadata, &m); err != nil {
		return fmt.Errorf("chunk metadata: %w", err)
	}
	if m.Situation == nil {
		return nil
	}
	if m.Situation.Schema != "situation-v1" || m.Situation.ID == "" || strings.TrimSpace(m.Situation.AnswerText) == "" || len(m.Situation.SourceSpans) == 0 {
		return fmt.Errorf("invalid or unsupported situation metadata")
	}
	for _, span := range m.Situation.SourceSpans {
		if span.DocumentSHA256 == "" || span.BlockID == "" || span.End <= span.Start || span.Start < 0 || len([]rune(span.Quote)) != span.End-span.Start {
			return fmt.Errorf("invalid situation source span")
		}
	}
	r.SituationID = m.Situation.ID
	r.AnswerText = m.Situation.AnswerText
	r.SourceSpans = m.Situation.SourceSpans
	for _, span := range r.SourceSpans {
		if !strings.Contains(r.Text, span.Quote) || !strings.Contains(r.AnswerText, span.Quote) {
			return fmt.Errorf("situation evidence absent from search or answer representation")
		}
	}
	return nil
}

func DocumentHash(markdown string) string {
	h := sha256.Sum256([]byte(markdown))
	return hex.EncodeToString(h[:])
}

// Verify against the immutable source, never merely against an annotation quote.
func ValidateSourceSpans(r SearchResult, markdown string) error {
	return validateSpans(r, DocumentHash(markdown), []rune(markdown))
}

func validateSpans(r SearchResult, documentHash string, runes []rune) error {
	for _, span := range r.SourceSpans {
		if span.DocumentSHA256 != documentHash || span.Start < 0 || span.End > len(runes) || span.End <= span.Start || string(runes[span.Start:span.End]) != span.Quote {
			return fmt.Errorf("source span mismatch for situation %s", r.SituationID)
		}
	}
	return nil
}

type SearchResult struct {
	DocumentSHA256 string       `json:"document_sha256,omitempty"`
	SituationID    string       `json:"situation_id,omitempty"`
	AnswerText     string       `json:"answer_text,omitempty"`
	SourceSpans    []SourceSpan `json:"source_spans,omitempty"`
	ChunkID        int64
	DocumentID     int64
	Sequence       int
	// Score is cosine similarity, including when results are ordered by fusion.

	Title     string
	Section   string
	Text      string
	SourceURL string
	Score     float64
	BM25      *float64 `json:"bm25,omitempty"`
	RRF       *float64 `json:"rrf,omitempty"`
	Rerank    *float64 `json:"rerank,omitempty"`
}

type SearchOptions struct {
	QueryText string
	// Empty mode selects hybrid when QueryText is supplied; vector keeps the old ranking.
	Mode  string
	Trace *SearchTrace
}

// SearchTrace records the existing ranking path without changing candidate depth.
// Nil scores mean that a stage did not run for that candidate.
type SearchTrace struct {
	BeforeFilters []SearchResult `json:"before_filters"`
	Candidates    []SearchResult `json:"candidates"`
	Selected      []SearchResult `json:"selected"`
	Selection     string         `json:"selection"`
}

func traceResults(chunks []rankedChunk) []SearchResult {
	results := make([]SearchResult, len(chunks))
	for i, c := range chunks {
		results[i] = c.SearchResult
	}
	return results
}

type rankedChunk struct {
	SearchResult
	documentID int64
	sequence   int
	fusion     float64
}

func (r *Repository) Search(ctx context.Context, gameID string, query []float32, limit int) ([]SearchResult, error) {
	return r.SearchWithOptions(ctx, gameID, query, limit, SearchOptions{})
}

func (r *Repository) SearchWithOptions(ctx context.Context, gameID string, query []float32, limit int, options SearchOptions) ([]SearchResult, error) {
	if options.Mode != "" && options.Mode != "vector" && options.Mode != "hybrid" {
		return nil, fmt.Errorf("unsupported retrieval mode %q", options.Mode)
	}
	if !gameIDPattern.MatchString(gameID) {
		return nil, fmt.Errorf("invalid game ID %q: expected a stable lowercase slug", gameID)
	}
	if len(query) == 0 {
		return nil, fmt.Errorf("query embedding is empty")
	}
	if limit <= 0 {
		return nil, nil
	}
	index, path, err := r.index(ctx, gameID)
	if err != nil {
		return nil, err
	}
	chunks := make([]rankedChunk, len(index.chunks))
	for i, cached := range index.chunks {
		if len(cached.vector) != len(query) {
			return nil, fmt.Errorf("RAG embedding dimensions are %d, query embedding dimensions are %d", len(cached.vector), len(query))
		}
		chunk := rankedChunk{SearchResult: cached.result, documentID: cached.result.DocumentID, sequence: cached.result.Sequence}
		chunk.Score = cosineSimilarity(query, cached.vector)
		chunks[i] = chunk
	}
	if options.Trace != nil {
		options.Trace.BeforeFilters = traceResults(chunks)
	}
	sort.Slice(chunks, func(i, j int) bool {
		if chunks[i].Score != chunks[j].Score {
			return chunks[i].Score > chunks[j].Score
		}
		return earlier(chunks[i], chunks[j])
	})
	if options.Mode != "vector" && options.QueryText != "" {
		database, err := openReadOnly(path)
		if err != nil {
			return nil, err
		}
		chunks, err = fuseKeywords(ctx, database, chunks, options.QueryText, limit)
		database.Close()
		if err != nil {
			return nil, err
		}
	}
	if options.Trace != nil {
		candidates := chunks
		// With no lexical hits, only the bounded dense pool is a candidate set.
		if len(candidates) > max(20, limit*4) && candidates[0].RRF == nil {
			candidates = candidates[:max(20, limit*4)]
		}
		options.Trace.Candidates = traceResults(candidates)
		options.Trace.Selection = "rank_limit"
	}
	// Collapse alternate search views without summing alias contributions.
	seen := map[string]bool{}
	unique := chunks[:0]
	for _, chunk := range chunks {
		if chunk.SituationID != "" {
			if seen[chunk.SituationID] {
				continue
			}
			seen[chunk.SituationID] = true
		}
		unique = append(unique, chunk)
	}
	chunks = unique
	if len(chunks) > limit {
		chunks = chunks[:limit]
	}
	results := make([]SearchResult, len(chunks))
	for i, chunk := range chunks {
		results[i] = chunk.SearchResult
	}
	if options.Trace != nil {
		options.Trace.Selected = results
	}
	return results, nil
}

func earlier(a, b rankedChunk) bool {
	if a.documentID != b.documentID {
		return a.documentID < b.documentID
	}
	return a.sequence < b.sequence
}

func decodeVector(encoded []byte, dimensions int) ([]float32, error) {
	if len(encoded) != dimensions*4 {
		return nil, fmt.Errorf("invalid stored embedding size %d for %d dimensions", len(encoded), dimensions)
	}
	vector := make([]float32, dimensions)
	for index := range vector {
		vector[index] = math.Float32frombits(binary.LittleEndian.Uint32(encoded[index*4:]))
	}
	return vector, nil
}

func cosineSimilarity(left, right []float32) float64 {
	var dot, leftNorm, rightNorm float64
	for index := range left {
		a, b := float64(left[index]), float64(right[index])
		dot += a * b
		leftNorm += a * a
		rightNorm += b * b
	}
	if leftNorm == 0 || rightNorm == 0 {
		return 0
	}
	return dot / math.Sqrt(leftNorm*rightNorm)
}
