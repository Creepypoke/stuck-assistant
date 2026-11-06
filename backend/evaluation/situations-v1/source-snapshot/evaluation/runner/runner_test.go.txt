package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/walkthrough-assistance/backend/ingestion"
	"github.com/walkthrough-assistance/backend/storage/rag"
)

func TestCacheKeyBindsExactInputAndProfile(t *testing.T) {
	p := Profile{Model: "embedding", Revision: "1", GGUFHash: "a", Dimensions: 2, InputFormat: "raw"}
	key := CacheKey(p, "A")
	for _, input := range []string{"a", "A ", "task: search | A"} {
		if key == CacheKey(p, input) {
			t.Fatal("input collision")
		}
	}
	changes := []Profile{p, p, p, p, p}
	changes[0].Revision = "2"
	changes[1].GGUFHash = "b"
	changes[2].Dimensions = 3
	changes[3].InputFormat = "prefixed"
	changes[4].Model = "other"
	for _, v := range changes {
		if key == CacheKey(v, "A") {
			t.Fatal("profile collision")
		}
	}
}
func TestValidationAndStageDiagnosis(t *testing.T) {
	for _, cases := range [][]Case{nil, {{ID: "../escape", Question: "q"}}, {{ID: "a", Question: "q"}, {ID: "a", Question: "q"}}, {{ID: "a"}}} {
		if ValidateCases(cases) == nil {
			t.Fatal("accepted invalid cases")
		}
	}
	if e := ValidateCases([]Case{{ID: "a", Question: "q"}, {ID: "b", Question: "r"}, {ID: "c", Question: "s"}}); e != nil {
		t.Fatal(e)
	}
	c := Case{Relevant: []int{1, 2}}
	first := []rag.SearchResult{{DocumentID: 1, Sequence: 1}}
	both := append(append([]rag.SearchResult{}, first...), rag.SearchResult{DocumentID: 1, Sequence: 2})
	if Diagnose(c, first, first) != "candidate_loss" || Diagnose(c, both, first) != "context_selection_loss" || Diagnose(c, both, both) != "generation_review_required" {
		t.Fatal("incomplete evidence scored as sufficient")
	}
	no := false
	c.SourcePresent = &no
	if Diagnose(c, both, both) != "source_missing" {
		t.Fatal("source failure not classified")
	}
}
func TestRunnerRetrievalLocalOracleResumeAndManifest(t *testing.T) {
	root := t.TempDir()
	repo, _ := rag.NewRepository(root)
	saved, e := repo.Save(context.Background(), ingestion.Document{GameID: "test-game", Title: "Guide", Markdown: "The code is 0392", Chunks: []ingestion.Chunk{{Sequence: 0, Text: "The code is 0392"}}, Embeddings: [][]float32{{1, 0}}, Embedding: ingestion.EmbeddingInfo{Model: "embedding", Version: "1", Dimensions: 2}, ChunkingVersion: "v1"})
	if e != nil {
		t.Fatal(e)
	}
	gguf := filepath.Join(root, "fake.gguf")
	if e = os.WriteFile(gguf, []byte("fixture model"), 0600); e != nil {
		t.Fatal(e)
	}
	generation, embeddings := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/props":
			_ = json.NewEncoder(w).Encode(map[string]any{"model_path": gguf, "model_alias": "fixture", "model_ftype": "fixture", "build_info": "fixture", "default_generation_settings": map[string]any{"n_ctx": 4096}, "chat_template": "enable_thinking", "chat_template_caps": map[string]any{}})
		case "/v1/embeddings":
			embeddings++
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"index": 0, "embedding": []float32{1, 0}}}})
		case "/v1/chat/completions":
			generation++
			_, _ = w.Write([]byte(`{"choices":[{"finish_reason":"stop","message":{"content":"0392"}}],"usage":{"prompt_tokens":20,"completion_tokens":3}}`))
		default:
			http.Error(w, "unexpected", 404)
		}
	}))
	defer server.Close()
	cases := []Case{{ID: "one", Question: "code?", Evidence: []Evidence{{DocumentID: 1, Sequence: 0, TextSHA256: Hash([]byte("The code is 0392"))}}}}
	casePath := filepath.Join(root, "cases.json")
	if e = save(casePath, cases); e != nil {
		t.Fatal(e)
	}
	for _, kind := range []string{"retrieval", "local", "oracle"} {
		out := filepath.Join(root, kind)
		args := []string{"--kind", kind, "--cases", casePath, "--corpus", saved.RAGPath, "--game", "test-game", "--out", out, "--llama-url", server.URL, "--embedding-url", server.URL, "--model", "fixture", "--dimensions", "2", "--profiles", "default,strict-ru"}
		beforeGeneration, beforeEmbeddings := generation, embeddings
		if e = RunArgs(kind, args); e != nil {
			t.Fatal(e)
		}
		if kind == "retrieval" && generation != beforeGeneration {
			t.Fatal("retrieval invoked generation")
		}
		if kind == "oracle" && embeddings != beforeEmbeddings {
			t.Fatal("oracle invoked embedding")
		}
		var records []Record
		if e = read(filepath.Join(out, "results.json"), &records); e != nil {
			t.Fatal(e)
		}
		if records[0].Error != "" {
			t.Fatal(records[0].Error)
		}
		if kind == "local" && (len(records[0].Debug.Attempts) != 1 || records[0].Debug.Attempts[0].PromptTokens != 20) {
			t.Fatal("missing usage")
		}
		beforeGeneration, beforeEmbeddings = generation, embeddings
		if e = RunArgs(kind, args); e != nil {
			t.Fatal(e)
		}
		if generation != beforeGeneration || embeddings != beforeEmbeddings {
			t.Fatal("resume repeated inference")
		}
		args = append(args, "--seed", "99")
		if RunArgs(kind, args) == nil {
			t.Fatal("accepted changed manifest")
		}
	}
}

func TestSourceGoldSurvivesChunkingAndRequiresEveryFact(t *testing.T) {
	span := rag.SourceSpan{DocumentSHA256: "doc", BlockID: "b", Start: 10, End: 14, Quote: "0392"}
	other := rag.SourceSpan{DocumentSHA256: "doc", BlockID: "a", Start: 0, End: 4, Quote: "3695"}
	c := Case{Evidence: []Evidence{{SourceSpan: &span}, {SourceSpan: &other}}}
	first := rag.SearchResult{DocumentID: 1, Sequence: 900, SituationID: "b2", SourceSpans: []rag.SourceSpan{span}}
	if any, all := completeness(c, []rag.SearchResult{first}); !any || all {
		t.Fatal("compound evidence incorrectly complete")
	}
	// Two different chunks can satisfy source evidence without old sequence IDs.
	second := rag.SearchResult{DocumentID: 1, Sequence: 901, SituationID: "b1", SourceSpans: []rag.SourceSpan{other}}
	if _, all := completeness(c, []rag.SearchResult{first, second}); !all {
		t.Fatal("new chunk IDs lost evidence")
	}
	first.SourceSpans[0].DocumentSHA256 = "different-version"
	if match(Case{Evidence: []Evidence{{SourceSpan: &span}}}, first) {
		t.Fatal("accepted different source version")
	}
}
