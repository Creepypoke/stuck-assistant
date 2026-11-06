package rag

import (
	"context"
	"github.com/walkthrough-assistance/data-preparation/model"
	"testing"
)

func TestSearchReturnsMostSimilarChunks(t *testing.T) {
	t.Parallel()
	repository, err := NewRepository(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Save(context.Background(), model.Document{
		GameID: "dino-crisis-psx", GameName: "Dino Crisis", SourceURL: "https://example.com/guide",
		Title: "Guide", Markdown: "body",
		Chunks: []model.Chunk{
			{Title: "Guide", Section: "First area", Sequence: 0, Text: "Find the keycard near the entrance."},
			{Title: "Guide", Section: "Basement", Sequence: 1, Text: "Use the elevator after restoring power."},
		},
		Embeddings: [][]float32{{1, 0}, {0, 1}},
		Embedding:  model.EmbeddingInfo{Model: "test", Dimensions: 2, Version: "1"}, ChunkingVersion: "v1",
	}); err != nil {
		t.Fatal(err)
	}
	results, err := repository.Search(context.Background(), "dino-crisis-psx", []float32{0.9, 0.1}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Section != "First area" {
		t.Fatalf("unexpected results: %#v", results)
	}
}
