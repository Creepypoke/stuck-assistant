package rag

import (
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
)

type SearchResult struct {
	Title     string
	Section   string
	Text      string
	SourceURL string
	Score     float64
}

type SearchOptions struct {
	Beginning bool
}

type rankedChunk struct {
	SearchResult
	documentID int64
	sequence   int
}

func (r *Repository) Search(ctx context.Context, gameID string, query []float32, limit int) ([]SearchResult, error) {
	return r.SearchWithOptions(ctx, gameID, query, limit, SearchOptions{})
}

func (r *Repository) SearchWithOptions(ctx context.Context, gameID string, query []float32, limit int, options SearchOptions) ([]SearchResult, error) {
	if !gameIDPattern.MatchString(gameID) {
		return nil, fmt.Errorf("invalid game ID %q: expected a stable lowercase slug", gameID)
	}
	if len(query) == 0 {
		return nil, fmt.Errorf("query embedding is empty")
	}
	if limit <= 0 {
		return nil, nil
	}
	path := filepath.Join(r.root, gameID+".rag")
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("knowledge for game %q is not installed", gameID)
		}
		return nil, fmt.Errorf("stat RAG file: %w", err)
	}
	database, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open RAG file: %w", err)
	}
	defer database.Close()

	rows, err := database.QueryContext(ctx, `
		SELECT c.title, c.section, c.text, d.source_url, e.dimensions, e.vector, c.document_id, c.sequence
		FROM embeddings e
		JOIN chunks c ON c.id = e.chunk_id
		JOIN documents d ON d.id = c.document_id`)
	if err != nil {
		return nil, fmt.Errorf("read RAG chunks: %w", err)
	}
	defer rows.Close()
	var chunks []rankedChunk
	for rows.Next() {
		var chunk rankedChunk
		var dimensions int
		var encoded []byte
		if err := rows.Scan(&chunk.Title, &chunk.Section, &chunk.Text, &chunk.SourceURL, &dimensions, &encoded, &chunk.documentID, &chunk.sequence); err != nil {
			return nil, fmt.Errorf("scan RAG chunk: %w", err)
		}
		if dimensions != len(query) {
			return nil, fmt.Errorf("RAG embedding dimensions are %d, query embedding dimensions are %d", dimensions, len(query))
		}
		vector, err := decodeVector(encoded, dimensions)
		if err != nil {
			return nil, err
		}
		chunk.Score = cosineSimilarity(query, vector)
		chunks = append(chunks, chunk)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate RAG chunks: %w", err)
	}
	if options.Beginning && len(chunks) > 0 {
		// Prefer the first document and its earliest chunks, never a later
		// walkthrough episode that happens to be semantically similar.
		sort.Slice(chunks, func(i, j int) bool { return earlier(chunks[i], chunks[j]) })
		firstDocument := chunks[0].documentID
		var results []SearchResult
		for _, chunk := range chunks {
			if chunk.documentID == firstDocument && len(results) < limit && chunk.sequence < 2 {
				results = append(results, chunk.SearchResult)
			}
		}
		if len(results) > 0 {
			return results, nil
		}
	}
	sort.Slice(chunks, func(i, j int) bool {
		if chunks[i].Score != chunks[j].Score {
			return chunks[i].Score > chunks[j].Score
		}
		return earlier(chunks[i], chunks[j])
	})
	if len(chunks) > limit {
		chunks = chunks[:limit]
	}
	results := make([]SearchResult, len(chunks))
	for i, chunk := range chunks {
		results[i] = chunk.SearchResult
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
