package query

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/walkthrough-assistance/backend/embeddings/llamacpp"
	"github.com/walkthrough-assistance/backend/ingestion"
	"github.com/walkthrough-assistance/backend/storage/rag"
)

func TestDiagnosticsRetrievalOracleRetryAndFailure(t *testing.T) {
	root := t.TempDir()
	repo, _ := rag.NewRepository(root)
	_, err := repo.Save(context.Background(), ingestion.Document{GameID: "test-game", Title: "Guide", Markdown: "0392", Chunks: []ingestion.Chunk{{Sequence: 0, Text: "The B2 code is 0392"}}, Embeddings: [][]float32{{1, 0}}, Embedding: ingestion.EmbeddingInfo{Model: "embedding", Version: "1", Dimensions: 2}, ChunkingVersion: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	calls, embedCalls := 0, 0
	fail := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/embeddings" {
			embedCalls++
			_, _ = w.Write([]byte(`{"data":[{"index":0,"embedding":[1,0]}]}`))
			return
		}
		calls++
		if fail {
			http.Error(w, "failed", 503)
			return
		}
		var payload struct {
			MaxTokens int `json:"max_tokens"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		if payload.MaxTokens == 256 {
			_, _ = w.Write([]byte(`{"choices":[{"finish_reason":"length","message":{"content":"0"}}],"usage":{"prompt_tokens":10,"completion_tokens":256}}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"finish_reason":"stop","message":{"content":"0392"}}],"usage":{"prompt_tokens":10,"completion_tokens":4}}`))
	}))
	defer server.Close()
	provider, _ := llamacpp.New(llamacpp.Config{BaseURL: server.URL, Model: "embedding", Version: "1", Dimensions: 2})
	s := &Service{repository: repo, embeddings: provider, client: server.Client(), config: Config{LLMURL: server.URL, LLMModel: "test", ResultLimit: 1, RetrievalMode: "hybrid"}}
	req := Request{GameID: "test-game", Question: "B2 code?"}
	debug, _, _, err := s.RetrieveDebug(context.Background(), req)
	if err != nil || calls != 0 || embedCalls != 1 || len(debug.Search.Candidates) != 1 || debug.Search.Selected[0].BM25 == nil || debug.Search.Selected[0].RRF == nil {
		t.Fatalf("retrieval trace: %+v err=%v calls=%d", debug, err, calls)
	}
	gold := debug.Search.Selected
	debug, _, answer, err := s.OracleDebug(context.Background(), req, gold)
	if err != nil || answer != "0392" || embedCalls != 1 || len(debug.Attempts) != 2 || debug.Attempts[0].FinishReason != "length" || debug.Attempts[1].CompletionTokens != 4 || debug.ContextMode != "oracle" {
		t.Fatalf("oracle/retry: %+v %v", debug, err)
	}
	fail = true
	debug, _, _, err = s.AskDebug(context.Background(), req)
	if err == nil || debug.FailureReason != "generation_error" || len(debug.RetrievedChunks) != 1 || len(debug.ModelRequest) == 0 || len(debug.Attempts) != 1 || debug.Attempts[0].Error == "" || debug.StageMS["total"] <= 0 {
		t.Fatalf("lost failure trace: %+v %v", debug, err)
	}
	if _, _, _, err = s.AskDebug(context.Background(), Request{}); err == nil {
		t.Fatal("accepted invalid request")
	}
}

func TestBeginningUsesRankedRetrieval(t *testing.T) {
	root := t.TempDir()
	repo, err := rag.NewRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.Save(context.Background(), ingestion.Document{
		GameID: "test-game", Title: "Guide", Markdown: "Guide",
		Chunks: []ingestion.Chunk{
			{Sequence: 0, Text: "About this guide."},
			{Sequence: 1, Text: "Controller settings."},
			{Sequence: 4, Text: "At the beginning of the game, approach Gail and open the green door."},
		},
		Embeddings: [][]float32{{0, 1}, {0.1, 1}, {1, 0}},
		Embedding:  ingestion.EmbeddingInfo{Model: "embedding", Version: "1", Dimensions: 2}, ChunkingVersion: "v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			t.Errorf("unexpected generation request: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"index":0,"embedding":[1,0]}]}`))
	}))
	defer server.Close()
	provider, err := llamacpp.New(llamacpp.Config{BaseURL: server.URL, Model: "embedding", Version: "1", Dimensions: 2})
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"vector", "hybrid"} {
		t.Run(mode, func(t *testing.T) {
			s := &Service{repository: repo, embeddings: provider, config: Config{ResultLimit: 1, RetrievalMode: mode}}
			for _, question := range []string{"Куда идти в начале игры?", "Я только получил управление Региной. Что сделать первым делом?", "What is the first thing to do at the beginning of the game?"} {
				debug, _, _, err := s.RetrieveDebug(context.Background(), Request{GameID: "test-game", Question: question})
				if err != nil {
					t.Fatal(err)
				}
				if debug.Search.Selection != "rank_limit" || len(debug.Search.Selected) != 1 || debug.Search.Selected[0].Sequence != 4 {
					t.Fatalf("%q: unexpected selection: %+v", question, debug.Search)
				}
				if mode == "hybrid" && question == "What is the first thing to do at the beginning of the game?" && debug.Search.Selected[0].RRF == nil {
					t.Fatal("beginning question bypassed hybrid fusion")
				}
			}
		})
	}
}
