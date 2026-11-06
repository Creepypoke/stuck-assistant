package service

import (
	"context"
	"testing"
	"time"

	"github.com/walkthrough-assistance/backend/ingestion"
)

type fakeProvider struct{}

func (fakeProvider) Download(_ context.Context, source ingestion.Source) (ingestion.RawDocument, error) {
	return ingestion.RawDocument{URL: source.URL + "/final", Content: []byte("html"), DownloadedAt: time.Unix(1, 0)}, nil
}

type fakeParser struct{}

func (fakeParser) Parse(context.Context, ingestion.RawDocument) (ingestion.ParsedDocument, error) {
	return ingestion.ParsedDocument{Title: "Guide", Markdown: "Body", Metadata: map[string]string{"language": "en"}}, nil
}

type fakeChunker struct{}

func (fakeChunker) Chunk(ingestion.ParsedDocument) ([]ingestion.Chunk, error) {
	return []ingestion.Chunk{{Title: "Guide", Section: "Start", Text: "Body"}}, nil
}
func (fakeChunker) Version() string { return "test-v1" }

type fakeEmbeddings struct{}

func (fakeEmbeddings) EmbedBatch(context.Context, []string) ([][]float32, error) {
	return [][]float32{{1, 2}}, nil
}
func (fakeEmbeddings) Info() ingestion.EmbeddingInfo {
	return ingestion.EmbeddingInfo{Model: "test", Dimensions: 2, Version: "1"}
}

type captureRepository struct{ document ingestion.Document }

func (repository *captureRepository) Save(_ context.Context, document ingestion.Document) (ingestion.Result, error) {
	repository.document = document
	return ingestion.Result{DocumentID: 7, ChunkCount: len(document.Chunks), RAGPath: "game.rag"}, nil
}

func TestIngestRunsPipelineAndPassesCompleteDocumentToRepository(t *testing.T) {
	t.Parallel()
	repository := &captureRepository{}
	service, err := New(Dependencies{
		Provider: fakeProvider{}, Parser: fakeParser{}, Chunker: fakeChunker{},
		Embeddings: fakeEmbeddings{}, Repository: repository,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Ingest(context.Background(), Request{GameID: "test-game", GameName: "Test Game", URL: "https://example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if result.DocumentID != 7 || repository.document.SourceURL != "https://example.com/final" {
		t.Fatalf("unexpected result or document: %#v %#v", result, repository.document)
	}
	if repository.document.Embedding.Model != "test" || repository.document.ChunkingVersion != "test-v1" {
		t.Fatalf("version metadata missing: %#v", repository.document)
	}
}
