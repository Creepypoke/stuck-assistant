package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/walkthrough-assistance/backend/ingestion"
	ingestionservice "github.com/walkthrough-assistance/backend/ingestion/service"
)

type recordingIngestor struct {
	request ingestionservice.Request
}

func (ingestor *recordingIngestor) Ingest(_ context.Context, request ingestionservice.Request) (ingestion.Result, error) {
	ingestor.request = request
	return ingestion.Result{DocumentID: 42, ChunkCount: 7, RAGPath: "/tmp/test-game.rag"}, nil
}

func TestKnowledgeAddRunsIngestion(t *testing.T) {
	t.Parallel()
	ingestor := &recordingIngestor{}
	var receivedOptions Options
	command := NewCommandWithFactory(func(options Options) (Ingestor, error) {
		receivedOptions = options
		return ingestor, nil
	})
	output := new(bytes.Buffer)
	command.SetOut(output)
	command.SetArgs([]string{
		"knowledge", "add", "test-game", "https://example.com/guide",
		"--game-name", "Test Game", "--storage-dir", "/tmp/games",
	})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if ingestor.request.GameID != "test-game" || ingestor.request.GameName != "Test Game" ||
		ingestor.request.URL != "https://example.com/guide" {
		t.Fatalf("unexpected ingestion request: %#v", ingestor.request)
	}
	if receivedOptions.StorageDirectory != "/tmp/games" {
		t.Fatalf("unexpected options: %#v", receivedOptions)
	}
	if receivedOptions.EmbeddingModel != "embedding" || receivedOptions.ChunkTokens != 300 || receivedOptions.OverlapTokens != 40 {
		t.Fatalf("unexpected default embedding options: %#v", receivedOptions)
	}
	if got := output.String(); !strings.Contains(got, "document_id: 42") || !strings.Contains(got, "chunks: 7") ||
		!strings.Contains(got, "path: /tmp/test-game.rag") {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestKnowledgeAddRequiresGameAndURL(t *testing.T) {
	t.Parallel()
	factoryCalled := false
	command := NewCommandWithFactory(func(Options) (Ingestor, error) {
		factoryCalled = true
		return &recordingIngestor{}, nil
	})
	command.SetArgs([]string{"knowledge", "add", "test-game"})

	if err := command.Execute(); err == nil {
		t.Fatal("expected argument validation error")
	}
	if factoryCalled {
		t.Fatal("factory must not be called for invalid arguments")
	}
}

func TestResolveExecutableRelativePath(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	expected := filepath.Join(filepath.Dir(executable), "data", "games")
	if actual := resolveExecutableRelativePath(filepath.Join("data", "games")); actual != expected {
		t.Fatalf("relative path = %q, want %q", actual, expected)
	}
	if actual := resolveExecutableRelativePath("/tmp/games"); actual != "/tmp/games" {
		t.Fatalf("absolute path = %q", actual)
	}
}

func TestResolveExecutableRelativeCommand(t *testing.T) {
	if actual := resolveExecutableRelativeCommand("python"); actual != "python" {
		t.Fatalf("command name = %q, want python", actual)
	}
	if actual := resolveExecutableRelativeCommand("--format"); actual != "--format" {
		t.Fatalf("option = %q, want --format", actual)
	}
	paths := resolveExecutableRelativePaths([]string{"./parser.py", "--format", "markdown"})
	if paths[1] != "--format" || paths[2] != "markdown" || !filepath.IsAbs(paths[0]) {
		t.Fatalf("unexpected resolved arguments: %#v", paths)
	}
}

func TestKnowledgeAddWiresCompletePipeline(t *testing.T) {
	t.Parallel()
	walkthroughServer := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "text/html")
		fmt.Fprint(response, "<html><body><h1>Guide</h1><p>Open the gate.</p></body></html>")
	}))
	defer walkthroughServer.Close()

	embeddingServer := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		var payload struct {
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			http.Error(response, err.Error(), http.StatusBadRequest)
			return
		}
		data := make([]map[string]any, len(payload.Input))
		for index := range payload.Input {
			data[index] = map[string]any{"index": index, "embedding": []float32{1, 2}}
		}
		response.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(response).Encode(map[string]any{"data": data})
	}))
	defer embeddingServer.Close()

	storageDirectory := t.TempDir()
	command := NewCommand()
	command.SetOut(new(bytes.Buffer))
	command.SetArgs([]string{
		"knowledge", "add", "test-game", walkthroughServer.URL,
		"--parser", os.Args[0],
		"--parser-arg=-test.run=TestCLIParserHelper",
		"--parser-arg=--",
		"--parser-arg=cli-parser-helper",
		"--embedding-url", embeddingServer.URL,
		"--embedding-model", "test-model",
		"--embedding-version", "test-version",
		"--embedding-dimensions", strconv.Itoa(2),
		"--storage-dir", storageDirectory,
	})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(storageDirectory, "test-game.rag")); err != nil {
		t.Fatalf("expected RAG file: %v", err)
	}
}

func TestKnowledgeAddReadsLocalFile(t *testing.T) {
	t.Parallel()
	sourcePath := filepath.Join(t.TempDir(), "guide.html")
	if err := os.WriteFile(sourcePath, []byte("<html><body><p>Open the gate.</p></body></html>"), 0o600); err != nil {
		t.Fatal(err)
	}
	embeddingServer := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		var payload struct {
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			http.Error(response, err.Error(), http.StatusBadRequest)
			return
		}
		data := make([]map[string]any, len(payload.Input))
		for index := range payload.Input {
			data[index] = map[string]any{"index": index, "embedding": []float32{1, 2}}
		}
		_ = json.NewEncoder(response).Encode(map[string]any{"data": data})
	}))
	defer embeddingServer.Close()

	storageDirectory := t.TempDir()
	command := NewCommand()
	command.SetOut(new(bytes.Buffer))
	command.SetArgs([]string{
		"knowledge", "add", "test-game", sourcePath,
		"--parser", os.Args[0],
		"--parser-arg=-test.run=TestCLIParserHelper",
		"--parser-arg=--",
		"--parser-arg=cli-parser-helper",
		"--embedding-url", embeddingServer.URL,
		"--embedding-model", "test-model",
		"--embedding-version", "test-version",
		"--embedding-dimensions", strconv.Itoa(2),
		"--storage-dir", storageDirectory,
	})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(storageDirectory, "test-game.rag")); err != nil {
		t.Fatalf("expected RAG file: %v", err)
	}
}

func TestCLIParserHelper(t *testing.T) {
	if len(os.Args) < 2 || os.Args[len(os.Args)-1] != "cli-parser-helper" {
		return
	}
	fmt.Print(`{"title":"Guide","markdown":"# Guide\n\nOpen the gate.","metadata":{"language":"en"}}`)
	os.Exit(0)
}
