package rag

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/walkthrough-assistance/data-preparation/pipeline"
)

// Real producer/consumer contract: prepare -> HTTP embeddings -> SQLite -> backend search.
func TestPreparedFileReadableByBackend(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "guide.txt")
	os.WriteFile(input, []byte("# B2\n\nB2 Key Chip: 0392."), 0600)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p struct {
			Input []string `json:"input"`
		}
		json.NewDecoder(r.Body).Decode(&p)
		data := []map[string]any{}
		for i := range p.Input {
			data = append(data, map[string]any{"index": i, "embedding": []float32{1, 0}})
		}
		json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	defer server.Close()
	c := pipeline.Config{GameID: "test-game", GameName: "Test", Input: input, Artifacts: filepath.Join(dir, "artifacts"), Output: filepath.Join(dir, "test-game.rag"), MaxInputBytes: 10000, MaxBatchChars: 12000, RequireReview: true, EmbeddingProvider: "test", Providers: map[string]pipeline.Provider{"test": {Type: "openai", BaseURL: server.URL, Model: "test", Version: "1", Dimensions: 2, Timeout: "1s"}}}
	ctx := context.Background()
	if e := pipeline.Prepare(ctx, c); e != nil {
		t.Fatal(e)
	}
	if e := pipeline.Build(ctx, c); e != nil {
		t.Fatal(e)
	}
	r, _ := NewRepository(dir)
	results, e := r.SearchWithOptions(ctx, "test-game", []float32{1, 0}, 1, SearchOptions{QueryText: "0392"})
	if e != nil || len(results) != 1 || results[0].AnswerText != "B2 Key Chip: 0392." || results[0].SituationID == "" || len(results[0].SourceSpans) != 1 {
		t.Fatalf("incompatible file: %+v, %v", results, e)
	}
}
