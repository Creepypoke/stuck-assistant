// Package runner implements the local P0 diagnostic protocol shared by both CLIs.
package runner

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/walkthrough-assistance/backend/embeddings/llamacpp"
	"github.com/walkthrough-assistance/backend/query"
	"github.com/walkthrough-assistance/backend/storage/rag"
)

type Case struct {
	ID            string     `json:"id"`
	Question      string     `json:"question"`
	Relevant      []int      `json:"relevant_sequences"` // Legacy single-document gold, retained for regression.
	Evidence      []Evidence `json:"evidence,omitempty"`
	Expected      string     `json:"expected"`
	SourcePresent *bool      `json:"source_present,omitempty"`
	ParsedPresent *bool      `json:"parsed_present,omitempty"`
}
type Evidence struct {
	DocumentID int64           `json:"document_id"`
	Sequence   int             `json:"sequence"`
	TextSHA256 string          `json:"text_sha256"`
	SourceSpan *rag.SourceSpan `json:"source_span,omitempty"`
}
type Profile struct {
	Model       string `json:"model"`
	Revision    string `json:"revision"`
	GGUFHash    string `json:"gguf_sha256"`
	Dimensions  int    `json:"dimensions"`
	InputFormat string `json:"input_format"`
}
type Cache struct {
	Profile Profile              `json:"profile"`
	Vectors map[string][]float32 `json:"vectors"`
}

func Hash(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func CacheKey(profile Profile, input string) string {
	b, _ := json.Marshal(struct {
		Profile Profile
		Input   string
	}{profile, input})
	return Hash(b)
}
func fileHash(path string) (string, error) {
	f, e := os.Open(path)
	if e != nil {
		return "", e
	}
	defer f.Close()
	h := sha256.New()
	_, e = io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil)), e
}
func read(path string, v any) error {
	b, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	return json.Unmarshal(b, v)
}
func save(path string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	return atomic(path, b)
}
func atomic(path string, b []byte) error {
	if e := os.WriteFile(path+".partial", b, 0600); e != nil {
		return e
	}
	return os.Rename(path+".partial", path)
}
func frozen(path string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	old, e := os.ReadFile(path)
	if e == nil {
		if string(old) != string(b) {
			return fmt.Errorf("%s changed; choose a fresh --out", path)
		}
		return nil
	}
	if !os.IsNotExist(e) {
		return e
	}
	return atomic(path, b)
}
func props(ctx context.Context, endpoint string) (map[string]any, error) {
	req, e := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(endpoint, "/")+"/props", nil)
	if e != nil {
		return nil, e
	}
	client := http.Client{Timeout: 30 * time.Second}
	res, e := client.Do(req)
	if e != nil {
		return nil, e
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("/props: %s", res.Status)
	}
	var p map[string]any
	e = json.NewDecoder(res.Body).Decode(&p)
	return p, e
}

// Runtime identity is read from the server, including the exact template and defaults.
// Hashing the reported local file prevents a logical alias from identifying a run.
func identity(ctx context.Context, endpoint string, generation bool) (map[string]any, error) {
	p, e := props(ctx, endpoint)
	if e != nil {
		return nil, e
	}
	path, _ := p["model_path"].(string)
	if path == "" {
		return nil, fmt.Errorf("server /props must expose model_path")
	}
	h, e := fileHash(path)
	if e != nil {
		return nil, fmt.Errorf("hash server model %s: %w", path, e)
	}
	keys := []string{"model_path", "model_alias", "model_ftype", "build_info", "default_generation_settings"}
	if generation {
		keys = append(keys, "chat_template", "chat_template_caps")
		t, _ := p["chat_template"].(string)
		if !strings.Contains(t, "enable_thinking") {
			return nil, fmt.Errorf("chat template does not declare enable_thinking; verify non-thinking support first")
		}
	}
	out := map[string]any{"gguf_sha256": h, "endpoint": endpoint}
	for _, k := range keys {
		if v, ok := p[k]; ok {
			out[k] = v
		} else {
			return nil, fmt.Errorf("/props missing %s", k)
		}
	}
	return out, nil
}
func sourceIdentity() (map[string]string, error) {
	result := map[string]string{}
	// Hash all Go sources plus dependency/preset files, including uncommitted code.
	e := filepath.WalkDir(".", func(path string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "tmp" || d.Name() == "bin" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") || path == "go.mod" || path == "go.sum" {
			h, e := fileHash(path)
			if e != nil {
				return e
			}
			result[path] = h
		}
		return nil
	})
	return result, e
}
func list(s string) ([]string, error) {
	result := strings.Split(s, ",")
	seen := map[string]bool{}
	for i, v := range result {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			return nil, fmt.Errorf("empty or duplicate list value")
		}
		seen[v] = true
		result[i] = v
	}
	return result, nil
}

var safeID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

func ValidateCases(cases []Case) error {
	if len(cases) == 0 {
		return fmt.Errorf("empty case set")
	}
	seen := map[string]bool{}
	for _, c := range cases {
		if !safeID.MatchString(c.ID) || seen[c.ID] || strings.TrimSpace(c.Question) == "" {
			return fmt.Errorf("invalid/duplicate case ID or empty question: %q", c.ID)
		}
		seen[c.ID] = true
	}
	return nil
}
func match(c Case, r rag.SearchResult) bool {
	if len(c.Evidence) > 0 {
		for _, e := range c.Evidence {
			if e.SourceSpan != nil {
				if len(r.SourceSpans) == 0 && r.DocumentSHA256 == e.SourceSpan.DocumentSHA256 && strings.Contains(r.Text, e.SourceSpan.Quote) {
					return true
				}
				for _, span := range r.SourceSpans {
					if span.DocumentSHA256 == e.SourceSpan.DocumentSHA256 && span.Start <= e.SourceSpan.Start && span.End >= e.SourceSpan.End {
						return true
					}
				}
				continue
			}
			if e.DocumentID == r.DocumentID && e.Sequence == r.Sequence && e.TextSHA256 == Hash([]byte(r.Text)) {
				return true
			}
		}
		return false
	}
	for _, s := range c.Relevant {
		if s == r.Sequence {
			return true
		}
	}
	return false
}
func completeness(c Case, results []rag.SearchResult) (bool, bool) {
	required := len(c.Evidence)
	if required == 0 {
		required = len(c.Relevant)
	}
	if required == 0 {
		return false, false
	}
	found := 0
	if len(c.Evidence) > 0 {
		for _, gold := range c.Evidence {
			for _, r := range results {
				if match(Case{Evidence: []Evidence{gold}}, r) {
					found++
					break
				}
			}
		}
	} else {
		for _, seq := range c.Relevant {
			for _, r := range results {
				if r.Sequence == seq {
					found++
					break
				}
			}
		}
	}
	return found > 0, found >= required
}

type Record struct {
	Case              string          `json:"case"`
	Mode              string          `json:"mode"`
	Profile           string          `json:"profile"`
	ContextMode       string          `json:"context_mode"`
	Answer            string          `json:"answer"`
	Expected          string          `json:"expected"`
	Error             string          `json:"error,omitempty"`
	Debug             query.DebugInfo `json:"debug"`
	ContextHash       string          `json:"context_sha256"`
	PromptHash        string          `json:"prompt_sha256"`
	CandidateHit      bool            `json:"candidate_hit"`
	CandidateComplete bool            `json:"candidate_complete"`
	ContextHit        bool            `json:"context_hit"`
	ContextComplete   bool            `json:"context_complete"`
	Diagnosis         string          `json:"diagnosis"`
}

func Diagnose(c Case, candidates, selected []rag.SearchResult) string {
	if c.SourcePresent != nil && !*c.SourcePresent {
		return "source_missing"
	}
	if c.ParsedPresent != nil && !*c.ParsedPresent {
		return "parsing_loss"
	}
	if len(c.Relevant) == 0 && len(c.Evidence) == 0 {
		return "unanswerable_review_required"
	}
	_, all := completeness(c, candidates)
	if !all {
		return "candidate_loss"
	}
	_, all = completeness(c, selected)
	if !all {
		return "context_selection_loss"
	}
	return "generation_review_required"
}
func loadOracle(corpus string, cases []Case) (map[string][]rag.SearchResult, error) {
	db, e := sql.Open("sqlite", "file:"+corpus+"?mode=ro")
	if e != nil {
		return nil, e
	}
	defer db.Close()
	var all []rag.SearchResult
	rows, e := db.Query(`SELECT c.id,c.document_id,c.sequence,c.title,c.section,c.text,d.source_url,c.metadata,d.markdown FROM chunks c JOIN documents d ON d.id=c.document_id ORDER BY c.document_id,c.sequence`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	documents := map[string]string{}
	for rows.Next() {
		var r rag.SearchResult
		var metadata []byte
		var markdown string
		if e = rows.Scan(&r.ChunkID, &r.DocumentID, &r.Sequence, &r.Title, &r.Section, &r.Text, &r.SourceURL, &metadata, &markdown); e != nil {
			return nil, e
		}
		if e = rag.ApplySituation(&r, metadata); e != nil {
			return nil, e
		}
		r.DocumentSHA256 = rag.DocumentHash(markdown)
		if e = rag.ValidateSourceSpans(r, markdown); e != nil {
			return nil, e
		}
		documents[Hash([]byte(markdown))] = markdown
		all = append(all, r)
	}
	if e = rows.Err(); e != nil {
		return nil, e
	}
	result := map[string][]rag.SearchResult{}
	for _, c := range cases {
		seen := map[int64]bool{}
		for _, gold := range c.Evidence {
			if gold.SourceSpan != nil {
				span := gold.SourceSpan
				md, ok := documents[span.DocumentSHA256]
				runes := []rune(md)
				if !ok || span.Start < 0 || span.End > len(runes) || span.End <= span.Start || string(runes[span.Start:span.End]) != span.Quote {
					return nil, fmt.Errorf("stale source span: %s", c.ID)
				}
			}
			found := false
			for _, r := range all {
				if match(Case{Evidence: []Evidence{gold}}, r) {
					found = true
					if !seen[r.ChunkID] {
						result[c.ID] = append(result[c.ID], r)
						seen[r.ChunkID] = true
					}
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("uncovered oracle evidence: %s", c.ID)
			}
		}
	}
	return result, nil
}

func Run(defaultKind string) error { return RunArgs(defaultKind, os.Args[1:]) }

func RunArgs(defaultKind string, args []string) error {
	fs := flag.NewFlagSet("evaluation", flag.ContinueOnError)
	kind := fs.String("kind", defaultKind, "retrieval (no LLM), local, or oracle")
	casesPath := fs.String("cases", "evaluation/dino-crisis-fts/questions.json", "any nonempty case set")
	corpus := fs.String("corpus", "bin/data/games/dino-crisis-1.rag", "input SQLite corpus (snapshotted read-only)")
	game := fs.String("game", "dino-crisis-1", "game slug")
	out := fs.String("out", "tmp/p0-evaluation", "immutable manifest and resumable records")
	endpoint := fs.String("llama-url", "http://127.0.0.1:8081", "generation server")
	embeddingURL := fs.String("embedding-url", "http://127.0.0.1:8081", "embedding server")
	model := fs.String("model", "assistance", "generation alias (identity verified through /props)")
	embeddingModel := fs.String("embedding-model", "embedding", "embedding alias")
	revision := fs.String("embedding-revision", "1", "index embedding revision/profile")
	dimensions := fs.Int("dimensions", 768, "embedding dimensions")
	profilesFlag := fs.String("profiles", "default,strict-ru", "comma-separated prompt profiles")
	modesFlag := fs.String("modes", "hybrid", "vector and/or hybrid")
	seed := fs.Int("seed", 20261104, "fixed generation seed")
	contextTokens := fs.Int("context-tokens", 0, "optional whole-evidence context cap using generator tokenizer")
	limit := fs.Int("limit", 5, "production result limit")
	if e := fs.Parse(args); e != nil {
		return e
	}
	if *kind != "retrieval" && *kind != "local" && *kind != "oracle" {
		return fmt.Errorf("invalid --kind")
	}
	if *limit <= 0 || *dimensions <= 0 {
		return fmt.Errorf("limit/dimensions must be positive")
	}
	if !regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`).MatchString(*game) {
		return fmt.Errorf("invalid game slug")
	}
	profiles, e := list(*profilesFlag)
	if e != nil {
		return e
	}
	modes, e := list(*modesFlag)
	if e != nil {
		return e
	}
	for _, m := range modes {
		if m != "vector" && m != "hybrid" {
			return fmt.Errorf("invalid retrieval mode %q", m)
		}
	}
	if *kind == "retrieval" {
		profiles = []string{"default"}
	}
	var cases []Case
	if e = read(*casesPath, &cases); e != nil {
		return e
	}
	if e = ValidateCases(cases); e != nil {
		return e
	}
	corpusAbs, e := filepath.Abs(*corpus)
	if e != nil {
		return e
	}
	ctx := context.Background()
	manifest := map[string]any{"schema": "p0-v1", "kind": *kind, "game": *game, "profiles": profiles, "modes": modes, "seed": *seed, "limit": *limit, "temperature": 0.2, "max_tokens": 256, "retry_max_tokens": 512, "enable_thinking": false, "no_spoilers": true, "answer_length": "short", "latency": "cached embeddings; serial generation; first call may be cold"}
	raw, e := os.ReadFile(*casesPath)
	if e != nil {
		return e
	}
	manifest["context_token_budget"] = *contextTokens
	manifest["cases_sha256"] = Hash(raw)
	h, e := fileHash(corpusAbs)
	if e != nil {
		return e
	}
	manifest["corpus_sha256"] = h
	code, e := sourceIdentity()
	if e != nil {
		return e
	}
	manifest["source_sha256"] = code
	commit, e := exec.Command("git", "rev-parse", "HEAD").Output()
	if e != nil {
		return e
	}
	manifest["git_head"] = strings.TrimSpace(string(commit))
	embeddingIdentity, e := identity(ctx, *embeddingURL, false)
	if e != nil {
		return fmt.Errorf("embedding identity: %w", e)
	}
	manifest["embedding_runtime"] = embeddingIdentity
	profile := Profile{Model: *embeddingModel, Revision: *revision, GGUFHash: embeddingIdentity["gguf_sha256"].(string), Dimensions: *dimensions, InputFormat: "raw-text-v1/no-application-prefix"}
	manifest["embedding_profile"] = profile
	if *kind != "retrieval" {
		runtime, e := identity(ctx, *endpoint, true)
		if e != nil {
			return fmt.Errorf("generation identity: %w", e)
		}
		manifest["generation_runtime"] = runtime
		manifest["generation_model"] = *model
	}
	for _, d := range []string{*out, filepath.Join(*out, "corpus"), filepath.Join(*out, "records")} {
		if e = os.MkdirAll(d, 0755); e != nil {
			return e
		}
	}
	if e = frozen(filepath.Join(*out, "manifest.json"), manifest); e != nil {
		return e
	}
	if e = atomic(filepath.Join(*out, "questions.json"), raw); e != nil {
		return e
	}
	snapshot := filepath.Join(*out, "corpus", *game+".rag")
	if old, e := fileHash(snapshot); e == nil {
		if old != h {
			return fmt.Errorf("snapshot changed")
		}
	} else if os.IsNotExist(e) {
		b, e := os.ReadFile(corpusAbs)
		if e != nil {
			return e
		}
		if e = atomic(snapshot, b); e != nil {
			return e
		}
	} else {
		return e
	}
	oracle, e := loadOracle(snapshot, cases)
	if e != nil {
		return e
	}
	cache := Cache{Profile: profile, Vectors: map[string][]float32{}}
	cachePath := filepath.Join(*out, "query-embeddings.json")
	if e = read(cachePath, &cache); e != nil && !os.IsNotExist(e) {
		return e
	}
	if cache.Profile != profile {
		return fmt.Errorf("embedding cache profile mismatch")
	}
	provider, e := llamacpp.New(llamacpp.Config{BaseURL: *embeddingURL, Model: *embeddingModel, Version: *revision, Dimensions: *dimensions, Timeout: 5 * time.Minute})
	if e != nil {
		return e
	}
	// Populate exact-input cache even when repeated/paraphrased case IDs differ.
	if *kind != "oracle" {
		for _, c := range cases {
			key := CacheKey(profile, c.Question)
			v, ok := cache.Vectors[key]
			if ok {
				if len(v) != *dimensions {
					return fmt.Errorf("invalid cached vector for %s", c.ID)
				}
				continue
			}
			values, e := provider.EmbedBatch(ctx, []string{c.Question})
			if e != nil {
				return e
			}
			cache.Vectors[key] = values[0]
			if e = save(cachePath, cache); e != nil {
				return e
			}
		}
	}
	// Freeze the actual vector bytes: resumed runs must not silently change inputs.
	if e = frozen(filepath.Join(*out, "cache-manifest.json"), cache); e != nil {
		return e
	}
	cached := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Model string
			Input []string
		}
		if r.URL.Path != "/v1/embeddings" || json.NewDecoder(r.Body).Decode(&in) != nil || in.Model != profile.Model {
			http.Error(w, "invalid cached request", 400)
			return
		}
		rows := []any{}
		for i, input := range in.Input {
			v, ok := cache.Vectors[CacheKey(profile, input)]
			if !ok {
				http.Error(w, "uncached input", 400)
				return
			}
			rows = append(rows, map[string]any{"index": i, "embedding": v})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": rows})
	}))
	defer cached.Close()
	records := []Record{}
	failures := 0
	for _, mode := range modes {
		for _, p := range profiles {
			service, e := query.New(query.Config{StorageDirectory: filepath.Dir(snapshot), EmbeddingURL: cached.URL, EmbeddingModel: *embeddingModel, EmbeddingVersion: *revision, EmbeddingDimensions: *dimensions, LLMURL: *endpoint, LLMModel: *model, Timeout: 5 * time.Minute, ResultLimit: *limit, ContextTokenBudget: *contextTokens, RetrievalMode: mode, PromptProfile: p, GenerationSeed: seed})
			if e != nil {
				return e
			}
			for _, c := range cases {
				if *kind == "oracle" && len(oracle[c.ID]) == 0 {
					continue
				} // Unanswerable cases have no oracle answer.
				path := filepath.Join(*out, "records", c.ID+"-"+mode+"-"+p+".json")
				var r Record
				if e = read(path, &r); e == nil && r.Error == "" && r.Case == c.ID && r.Mode == mode && r.Profile == p {
					records = append(records, r)
					continue
				}
				req := query.Request{GameID: *game, Question: c.Question, NoSpoilers: true, AnswerLength: "short"}
				var debug query.DebugInfo
				var answer string
				switch *kind {
				case "retrieval":
					debug, _, answer, e = service.RetrieveDebug(ctx, req)
				case "oracle":
					debug, _, answer, e = service.OracleDebug(ctx, req, oracle[c.ID])
				default:
					debug, _, answer, e = service.AskDebug(ctx, req)
				}
				r = Record{Case: c.ID, Mode: mode, Profile: p, ContextMode: debug.ContextMode, Debug: debug, Answer: answer, Expected: c.Expected}
				if e != nil {
					r.Error = e.Error()
					failures++
				}
				b, _ := json.Marshal(debug.RetrievedChunks)
				r.ContextHash = Hash(b)
				r.PromptHash = Hash(debug.ModelRequest)
				r.CandidateHit, r.CandidateComplete = completeness(c, debug.Search.Candidates)
				r.ContextHit, r.ContextComplete = completeness(c, debug.Search.Selected)
				r.Diagnosis = Diagnose(c, debug.Search.Candidates, debug.Search.Selected)
				if *kind == "oracle" {
					r.Diagnosis = "oracle_generation_review_required"
				}
				if r.Error != "" {
					r.Diagnosis = debug.FailureReason
				}
				if e = save(path, r); e != nil {
					return e
				}
				records = append(records, r)
				if e = save(filepath.Join(*out, "results.json"), records); e != nil {
					return e
				}
				fmt.Printf("%s %s %s: %s error=%q\n", c.ID, mode, p, r.Diagnosis, r.Error)
			}
		}
	}
	if *kind == "oracle" && len(records) == 0 {
		return fmt.Errorf("oracle run has no verified evidence")
	}
	if e = save(filepath.Join(*out, "results.json"), records); e != nil {
		return e
	}
	summary := map[string]any{"records": len(records), "errors": failures, "answer_scoring": "manual review required; retrieval hits are not answer correctness"}
	for _, mode := range modes {
		answerable, hit, candidate, complete := 0, 0, 0, 0
		for _, r := range records {
			if r.Mode != mode || r.Profile != profiles[0] || len(cases) == 0 {
				continue
			}
			for _, c := range cases {
				if c.ID == r.Case && (len(c.Relevant) > 0 || len(c.Evidence) > 0) {
					answerable++
					if r.ContextHit {
						hit++
					}
					if r.CandidateComplete {
						candidate++
					}
					if r.ContextComplete {
						complete++
					}
				}
			}
		}
		summary[mode] = map[string]int{"answerable": answerable, "context_hit": hit, "candidate_complete": candidate, "context_complete": complete}
	}
	if e = save(filepath.Join(*out, "summary.json"), summary); e != nil {
		return e
	}
	if failures > 0 {
		return fmt.Errorf("%d failed cells; records saved for diagnosis/resume", failures)
	}
	return nil
}
