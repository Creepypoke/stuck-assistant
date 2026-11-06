package pipeline

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T) Config {
	t.Helper()
	dir := t.TempDir()
	input := filepath.Join(dir, "guide.html")
	if e := os.WriteFile(input, []byte(`<html><nav>advertisement</nav><article><h1>Game</h1><h2>Basement</h2><p>Чип B2: введите 0392.</p><ol><li>GREEN</li><li>GREEN</li><li>RED</li></ol><img alt="puzzle" src="x"></article></html>`), 0600); e != nil {
		t.Fatal(e)
	}
	return Config{GameID: "test-game", GameName: "Game", Input: input, Artifacts: filepath.Join(dir, "artifacts"), Output: filepath.Join(dir, "test-game.rag"), MaxInputBytes: 1 << 20, MaxBatchChars: 12000, RequireReview: true}
}
func TestPrepareBuildAndImmutableEvidence(t *testing.T) {
	c := fixture(t)
	ctx := context.Background()
	if e := Prepare(ctx, c); e != nil {
		t.Fatal(e)
	}
	a, e := Load(c.Artifacts)
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(a.Document, "advertisement") || !strings.Contains(a.Document, "0392") || strings.Count(a.Document, "GREEN") != 2 || len(a.Manifest.Warnings) == 0 {
		t.Fatalf("bad extraction: %+v", a)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			t.Errorf("path %s", r.URL.Path)
		}
		var req struct {
			Input []string `json:"input"`
		}
		if e := json.NewDecoder(r.Body).Decode(&req); e != nil {
			t.Error(e)
		}
		data := []map[string]any{}
		for i := len(req.Input) - 1; i >= 0; i-- {
			data = append(data, map[string]any{"index": i, "embedding": []float32{1, 2}})
		}
		json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	defer server.Close()
	c.EmbeddingProvider = "test"
	c.Providers = map[string]Provider{"test": {Type: "openai", BaseURL: server.URL + "/v1", Model: "test", Version: "v1", Dimensions: 2, Timeout: "5s"}}
	if e = Build(ctx, c); e != nil {
		t.Fatal(e)
	}
	db, e := sql.Open("sqlite", c.Output)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	var n int
	if e = db.QueryRow(`SELECT count(*) FROM chunks_fts WHERE chunks_fts MATCH '0392'`).Scan(&n); e != nil || n != 1 {
		t.Fatalf("FTS n=%d err=%v", n, e)
	}
	var m string
	if e = db.QueryRow(`SELECT metadata FROM chunks WHERE text LIKE '%0392%'`).Scan(&m); e != nil || !strings.Contains(m, `"situation-v1"`) {
		t.Fatalf("metadata: %s %v", m, e)
	}
	if Build(ctx, c) == nil || Prepare(ctx, c) == nil {
		t.Fatal("must not overwrite outputs")
	}
	// The persisted evidence may not be silently repaired after editing.
	a.Cards[0].SourceSpans[0].Quote = "invented"
	if e = WriteYAML(filepath.Join(c.Artifacts, "cards.yaml"), a.Cards); e != nil {
		t.Fatal(e)
	}
	if _, e = Load(c.Artifacts); e == nil {
		t.Fatal("tampered source accepted")
	}
}
func TestAIReviewAndSourceCoverage(t *testing.T) {
	for _, kind := range []string{"openai", "anthropic"} {
		t.Run(kind, func(t *testing.T) {
			c := fixture(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req map[string]any
				json.NewDecoder(r.Body).Decode(&req)
				msgs := req["messages"].([]any)
				text := msgs[len(msgs)-1].(map[string]any)["content"].(string)
				var blocks []Block
				if e := decode([]byte(text), &blocks); e != nil {
					t.Error(e)
				}
				cards := []Card{}
				for _, b := range blocks {
					cards = append(cards, Card{ID: "temporary", Task: b.Text, BlockIDs: []string{b.ID}})
				}
				y, _ := yamlBytes(map[string]any{"cards": cards})
				if kind == "anthropic" {
					json.NewEncoder(w).Encode(map[string]any{"stop_reason": "end_turn", "content": []map[string]string{{"type": "text", "text": string(y)}}})
				} else {
					json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"finish_reason": "stop", "message": map[string]string{"content": string(y)}}}})
				}
			}))
			defer server.Close()
			c.AnnotationProvider = "ai"
			c.Providers = map[string]Provider{"ai": {Type: kind, BaseURL: server.URL, Model: "annotation", Timeout: "5s", MaxOutputTokens: 4096}}
			if e := Prepare(context.Background(), c); e != nil {
				t.Fatal(e)
			}
			if Validate(c) == nil {
				t.Fatal("AI cards bypassed review")
			}
			a, e := Load(c.Artifacts)
			if e != nil {
				t.Fatal(e)
			}
			for i := range a.Cards {
				a.Cards[i].ValidationStatus = "reviewed"
			}
			WriteYAML(filepath.Join(c.Artifacts, "cards.yaml"), a.Cards)
			if e = Validate(c); e != nil {
				t.Fatal(e)
			}
			a.Cards[0].ExactCode = "0000"
			WriteYAML(filepath.Join(c.Artifacts, "cards.yaml"), a.Cards)
			if Validate(c) == nil {
				t.Fatal("unsupported code accepted")
			}
		})
	}
}
func TestConfigAndText(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yaml")
	text := "game_id: test-game\ngame_name: Test\ninput: guide.txt\nartifacts: artifacts\noutput: output.rag\n"
	os.WriteFile(p, []byte(text), 0600)
	cliPath := filepath.Join(dir, "cli.yaml")
	os.WriteFile(cliPath, []byte("providers: {}\n"), 0600)
	c, e := LoadConfig(cliPath, p)
	if e != nil || c.Input != filepath.Join(dir, "guide.txt") {
		t.Fatalf("config %+v %v", c, e)
	}
	os.WriteFile(p, []byte(text+"typo: true\n"), 0600)
	if _, e = LoadConfig(cliPath, p); e == nil {
		t.Fatal("unknown field accepted")
	}
	os.WriteFile(c.Input, []byte("# Начало\n\nКод: 0032.\n\nНажмите GREEN, GREEN, RED."), 0600)
	if e = Prepare(context.Background(), c); e != nil {
		t.Fatal(e)
	}
	os.WriteFile(filepath.Join(c.Artifacts, "document.txt"), []byte("changed"), 0600)
	if _, e = Load(c.Artifacts); e == nil {
		t.Fatal("modified document accepted")
	}
}
func TestRejectMissingBlocksAndBadVectors(t *testing.T) {
	c := fixture(t)
	if e := Prepare(context.Background(), c); e != nil {
		t.Fatal(e)
	}
	a, e := Load(c.Artifacts)
	if e != nil {
		t.Fatal(e)
	}
	a.Cards = a.Cards[1:]
	if validate(a, false) == nil {
		t.Fatal("missing source accepted")
	}
	for _, data := range []string{`{"data":[{"index":0,"embedding":[0,0]}]}`, `{"data":[{"index":1,"embedding":[1,2]}]}`, `{"data":[]}`} {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(data)) }))
		_, e := embed(context.Background(), Provider{Type: "openai", BaseURL: s.URL, Model: "e", Version: "v1", Dimensions: 2, Timeout: "1s"}, []string{"a"})
		s.Close()
		if e == nil {
			t.Fatal("bad vectors accepted")
		}
	}
}

func TestReviewedFactsCompactEvidence(t *testing.T) {
	c := fixture(t)
	if e := Prepare(context.Background(), c); e != nil {
		t.Fatal(e)
	}
	a, e := Load(c.Artifacts)
	if e != nil {
		t.Fatal(e)
	}
	for i := range a.Cards {
		card := &a.Cards[i]
		b := a.Blocks[i]
		if strings.Contains(b.Text, "0392") {
			card.ValidationStatus = "reviewed"
			card.Task = "Чип B2"
			card.Facts = []Fact{{Quote: "0392", BlockID: b.ID}}
			card.ExactCode = "0392"
		}
	}
	if e = validate(a, true); e != nil {
		t.Fatal(e)
	}
	if e = hydrate(&a); e != nil {
		t.Fatal(e)
	}
	found := false
	for _, entry := range a.Entries {
		if entry.AnswerText == "0392" {
			found = true
			sp := entry.SourceSpans[0]
			if string([]rune(a.Document)[sp.Start:sp.End]) != "0392" || !strings.Contains(entry.SearchText, "введите") {
				t.Fatal("compact evidence lost source or search text")
			}
		}
	}
	if !found {
		t.Fatal("no compact answer")
	}
}

func TestRepeatedActionsRemainInCompactAnswer(t *testing.T) {
	c := fixture(t)
	os.WriteFile(c.Input, []byte(`<article><p>GREEN GREEN RED</p></article>`), 0600)
	if e := Prepare(context.Background(), c); e != nil {
		t.Fatal(e)
	}
	a, e := Load(c.Artifacts)
	if e != nil {
		t.Fatal(e)
	}
	card := &a.Cards[0]
	card.ValidationStatus = "reviewed"
	card.Facts = []Fact{{Quote: "GREEN", BlockID: a.Blocks[0].ID}}
	card.OrderedActions = []string{"GREEN", "GREEN", "RED"}
	if e = hydrate(&a); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(a.Entries[0].AnswerText, "1. GREEN\n2. GREEN\n3. RED") {
		t.Fatalf("repeat lost: %s", a.Entries[0].AnswerText)
	}
}

func TestSharedCLIConfigAndIndependentWalkthroughs(t *testing.T) {
	dir := t.TempDir()
	cli := filepath.Join(dir, "cli.yaml")
	if e := os.WriteFile(cli, []byte("providers:\n  embedding:\n    type: openai\n    base_url: http://localhost:8081/v1\n    model: embedding\n    dimensions: 768\n    version: '1'\n"), 0600); e != nil {
		t.Fatal(e)
	}
	for _, game := range []string{"first-game", "second-game"} {
		jobDir := filepath.Join(dir, game)
		os.MkdirAll(jobDir, 0700)
		job := filepath.Join(jobDir, "walkthrough.yaml")
		text := "game_id: " + game + "\ngame_name: Test\ninput: source.txt\nartifacts: artifacts\noutput: game.rag\nembedding_provider: embedding\nrequire_review: false\n"
		os.WriteFile(job, []byte(text), 0600)
		c, e := LoadConfig(cli, job)
		if e != nil {
			t.Fatal(e)
		}
		if c.Input != filepath.Join(jobDir, "source.txt") || c.Artifacts != filepath.Join(jobDir, "artifacts") || c.GameID != game || c.RequireReview || c.Providers["embedding"].Timeout != "2m" {
			t.Fatalf("cross-config leakage: %+v", c)
		}
		os.WriteFile(job, []byte(text+"providers: {}\n"), 0600)
		if _, e := LoadConfig(cli, job); e == nil {
			t.Fatal("walkthrough must not define global providers")
		}
		os.WriteFile(job, []byte(strings.ReplaceAll(text, "embedding_provider: embedding", "embedding_provider: missing")), 0600)
		if _, e := LoadConfig(cli, job); e == nil {
			t.Fatal("unknown provider accepted")
		}
	}
	os.WriteFile(cli, []byte("providers: {}\ngame_id: misplaced\n"), 0600)
	if _, e := LoadConfig(cli, filepath.Join(dir, "first-game", "walkthrough.yaml")); e == nil {
		t.Fatal("CLI config accepted game-specific fields")
	}
}
