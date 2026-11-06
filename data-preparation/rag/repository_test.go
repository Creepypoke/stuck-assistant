package rag

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/walkthrough-assistance/data-preparation/model"
)

func TestSaveCreatesSelfContainedRAGFileAndFTSIndex(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	repository, err := NewRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	result, err := repository.Save(context.Background(), model.Document{
		GameID: "elden-ring", GameName: "ELDEN RING", SourceURL: "https://example.com/guide",
		Title: "Guide", RawHTML: []byte("<p>Stormveil Castle</p>"), Markdown: "# Guide\n\nStormveil Castle",
		Chunks:          []model.Chunk{{Title: "Guide", Section: "Castle", Sequence: 0, Text: "Find the hidden key in Stormveil Castle"}},
		Embeddings:      [][]float32{{0.1, 0.2, 0.3}},
		Embedding:       model.EmbeddingInfo{Model: "embedding-test", Dimensions: 3, Version: "1"},
		ChunkingVersion: "markdown-v1", DownloadedAt: time.Unix(1, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ChunkCount != 1 || result.RAGPath != filepath.Join(root, "elden-ring.rag") {
		t.Fatalf("unexpected result: %#v", result)
	}

	database, err := sql.Open("sqlite", result.RAGPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var count int
	if err := database.QueryRow(`SELECT count(*) FROM chunks_fts WHERE chunks_fts MATCH 'Stormveil'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("FTS result count = %d, want 1", count)
	}
	var gameID string
	if err := database.QueryRow("SELECT value FROM metadata WHERE key = 'game_id'").Scan(&gameID); err != nil {
		t.Fatal(err)
	}
	if gameID != "elden-ring" {
		t.Fatalf("game_id metadata = %q", gameID)
	}
}

func TestSaveRejectsIncompatibleEmbeddingMetadata(t *testing.T) {
	t.Parallel()
	repository, err := NewRepository(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	document := model.Document{
		GameID: "test-game", GameName: "Test", SourceURL: "https://example.com", Title: "Guide", Markdown: "body",
		Chunks: []model.Chunk{{Sequence: 0, Text: "body"}}, Embeddings: [][]float32{{1, 2}},
		Embedding: model.EmbeddingInfo{Model: "model-a", Dimensions: 2, Version: "1"}, ChunkingVersion: "v1",
	}
	if _, err := repository.Save(context.Background(), document); err != nil {
		t.Fatal(err)
	}
	document.Embedding.Model = "model-b"
	if _, err := repository.Save(context.Background(), document); err == nil {
		t.Fatal("expected incompatible metadata error")
	}
}
