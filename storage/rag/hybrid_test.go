package rag

import (
	"context"
	"github.com/walkthrough-assistance/backend/ingestion"
	"strings"
	"testing"
)

func TestHybridRetrievesExactItemBeyondVectorCandidates(t *testing.T) {
	repo, _ := NewRepository(t.TempDir())
	chunks := []ingestion.Chunk{}
	vectors := [][]float32{}
	for i := 0; i < 25; i++ {
		chunks = append(chunks, ingestion.Chunk{Sequence: i, Text: "An unrelated generator puzzle."})
		vectors = append(vectors, []float32{1, 0})
	}
	chunks = append(chunks, ingestion.Chunk{Sequence: 25, Text: "Use the Panel Key 2 to open this safe."})
	vectors = append(vectors, []float32{0, 1})
	_, err := repo.Save(context.Background(), ingestion.Document{GameID: "test-game", Title: "Guide", Markdown: "body", Chunks: chunks, Embeddings: vectors, Embedding: ingestion.EmbeddingInfo{Model: "test", Version: "1", Dimensions: 2}, ChunkingVersion: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"vector", "hybrid"} {
		got, err := repo.SearchWithOptions(context.Background(), "test-game", []float32{1, 0}, 5, SearchOptions{Mode: mode, QueryText: "Где использовать Panel Key 2?"})
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, r := range got {
			if r.Sequence == 25 {
				found = true
			}
		}
		if found != (mode == "hybrid") {
			t.Fatalf("mode=%s found=%v results=%+v", mode, found, got)
		}
		ids := map[int64]bool{}
		for _, r := range got {
			if ids[r.ChunkID] {
				t.Fatal("duplicate result")
			}
			ids[r.ChunkID] = true
		}
	}
}

func TestHybridNoLexicalMatchesPreservesVectorOrder(t *testing.T) {
	repo, _ := NewRepository(t.TempDir())
	_, err := repo.Save(context.Background(), ingestion.Document{GameID: "test-game", Title: "Guide", Markdown: "body", Chunks: []ingestion.Chunk{{Sequence: 0, Text: "First room"}, {Sequence: 1, Text: "Basement"}}, Embeddings: [][]float32{{0, 1}, {1, 0}}, Embedding: ingestion.EmbeddingInfo{Model: "test", Version: "1", Dimensions: 2}, ChunkingVersion: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"несуществующееслово", "что мне нужно сделать?", "\" OR NOT : * ( )"} {
		got, err := repo.SearchWithOptions(context.Background(), "test-game", []float32{1, 0}, 2, SearchOptions{QueryText: q})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 || got[0].Sequence != 1 || got[1].Sequence != 0 {
			t.Fatalf("query=%q got=%+v", q, got)
		}
	}
}

func TestKeywordExpressionTreatsOperatorsAsWords(t *testing.T) {
	got := keywordExpression(`Где Panel Key 2? Panel " OR text:GREEN - RED* (BLUE)`)
	for _, word := range []string{`"panel"`, `"key"`, `"2"`, `"green"`, `"red"`, `"blue"`} {
		if !strings.Contains(got, word) {
			t.Fatalf("missing %s in %s", word, got)
		}
	}
	if strings.Count(got, `"panel"`) != 1 || strings.Contains(got, ":") || strings.Contains(got, "*") {
		t.Fatal(got)
	}
	if got := keywordExpression("что мне нужно сделать?"); got != "" {
		t.Fatal(got)
	}
}
