package rag

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/walkthrough-assistance/data-preparation/model"
)

func saveGuide(t *testing.T, repository *Repository, section string) {
	t.Helper()
	if _, err := repository.Save(context.Background(), model.Document{
		GameID: "cached-game", GameName: "Cached", SourceURL: "https://example.com/guide",
		Title: "Guide", Markdown: "body",
		Chunks: []model.Chunk{
			{Title: "Guide", Section: section, Sequence: 0, Text: "Find the keycard near the entrance."},
			{Title: "Guide", Section: "Other", Sequence: 1, Text: "Use the elevator after restoring power."},
		},
		Embeddings: [][]float32{{1, 0}, {0, 1}},
		Embedding:  model.EmbeddingInfo{Model: "test", Dimensions: 2, Version: "1"}, ChunkingVersion: "v1",
	}); err != nil {
		t.Fatal(err)
	}
}

func topSection(t *testing.T, repository *Repository) string {
	t.Helper()
	results, err := repository.SearchWithOptions(context.Background(), "cached-game", []float32{1, 0}, 1, SearchOptions{QueryText: "keycard"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("unexpected results: %#v", results)
	}
	return results[0].Section
}

func TestSearchReusesLoadedIndex(t *testing.T) {
	root := t.TempDir()
	repository, _ := NewRepository(root)
	saveGuide(t, repository, "First")
	topSection(t, repository)
	path := filepath.Join(root, "cached-game.rag")
	first := repository.cache[path]
	if first == nil {
		t.Fatal("index was not cached")
	}
	topSection(t, repository)
	if repository.cache[path] != first {
		t.Fatal("unchanged file was reloaded")
	}
}

func TestSearchReloadsReplacedFile(t *testing.T) {
	root := t.TempDir()
	repository, _ := NewRepository(root)
	saveGuide(t, repository, "First")
	if got := topSection(t, repository); got != "First" {
		t.Fatalf("section = %q", got)
	}
	// Installing a new build replaces the file; stale chunks must not be served.
	if err := os.Remove(filepath.Join(root, "cached-game.rag")); err != nil {
		t.Fatal(err)
	}
	saveGuide(t, repository, "Rebuilt")
	if got := topSection(t, repository); got != "Rebuilt" {
		t.Fatalf("section after rebuild = %q", got)
	}
}

func TestSearchRejectsDimensionMismatchFromCache(t *testing.T) {
	repository, _ := NewRepository(t.TempDir())
	saveGuide(t, repository, "First")
	topSection(t, repository)
	if _, err := repository.Search(context.Background(), "cached-game", []float32{1, 0, 0}, 1); err == nil {
		t.Fatal("expected dimension mismatch error")
	}
}

func TestConcurrentSearchesShareIndex(t *testing.T) {
	repository, _ := NewRepository(t.TempDir())
	saveGuide(t, repository, "First")
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results, err := repository.SearchWithOptions(context.Background(), "cached-game", []float32{1, 0}, 2, SearchOptions{QueryText: "keycard"})
			if err != nil || len(results) != 2 || results[0].Section != "First" {
				t.Errorf("results = %#v, err = %v", results, err)
			}
		}()
	}
	wg.Wait()
}
