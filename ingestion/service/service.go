package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/walkthrough-assistance/backend/ingestion"
)

type Service struct {
	provider   ingestion.WalkthroughProvider
	parser     ingestion.DocumentParser
	chunker    ingestion.Chunker
	embeddings ingestion.EmbeddingProvider
	repository ingestion.Repository
	logger     *slog.Logger
}

type Dependencies struct {
	Provider   ingestion.WalkthroughProvider
	Parser     ingestion.DocumentParser
	Chunker    ingestion.Chunker
	Embeddings ingestion.EmbeddingProvider
	Repository ingestion.Repository
	Logger     *slog.Logger
}

type Request struct {
	GameID   string
	GameName string
	URL      string
}

func New(dependencies Dependencies) (*Service, error) {
	if dependencies.Provider == nil || dependencies.Parser == nil || dependencies.Chunker == nil ||
		dependencies.Embeddings == nil || dependencies.Repository == nil {
		return nil, fmt.Errorf("all ingestion dependencies are required")
	}
	logger := dependencies.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		provider: dependencies.Provider, parser: dependencies.Parser, chunker: dependencies.Chunker,
		embeddings: dependencies.Embeddings, repository: dependencies.Repository, logger: logger,
	}, nil
}

func (s *Service) Ingest(ctx context.Context, request Request) (ingestion.Result, error) {
	started := time.Now()
	if strings.TrimSpace(request.GameID) == "" || strings.TrimSpace(request.URL) == "" {
		return ingestion.Result{}, &ingestion.StageError{Stage: "validate", Err: ingestion.ErrInvalidSource}
	}

	raw, err := s.provider.Download(ctx, ingestion.Source{URL: request.URL})
	if err != nil {
		return ingestion.Result{}, &ingestion.StageError{Stage: "download", Err: err}
	}
	parsed, err := s.parser.Parse(ctx, raw)
	if err != nil {
		return ingestion.Result{}, &ingestion.StageError{Stage: "parse", Err: err}
	}
	chunks, err := s.chunker.Chunk(parsed)
	if err != nil {
		return ingestion.Result{}, &ingestion.StageError{Stage: "chunk", Err: err}
	}
	texts := make([]string, len(chunks))
	for index := range chunks {
		texts[index] = chunks[index].Text
	}
	vectors, err := s.embeddings.EmbedBatch(ctx, texts)
	if err != nil {
		return ingestion.Result{}, &ingestion.StageError{Stage: "embed", Err: err}
	}
	if len(vectors) != len(chunks) {
		return ingestion.Result{}, &ingestion.StageError{
			Stage: "embed", Err: fmt.Errorf("provider returned %d vectors for %d chunks", len(vectors), len(chunks)),
		}
	}
	documentTitle := parsed.Title
	if documentTitle == "" && len(chunks) > 0 {
		documentTitle = chunks[0].Title
	}

	result, err := s.repository.Save(ctx, ingestion.Document{
		GameID: request.GameID, GameName: request.GameName, SourceURL: raw.URL,
		Title: documentTitle, RawHTML: raw.Content, Markdown: parsed.Markdown,
		ParserMetadata: parsed.Metadata, Chunks: chunks, Embeddings: vectors,
		Embedding: s.embeddings.Info(), ChunkingVersion: s.chunker.Version(), DownloadedAt: raw.DownloadedAt,
	})
	if err != nil {
		return ingestion.Result{}, &ingestion.StageError{Stage: "store", Err: err}
	}
	s.logger.InfoContext(ctx, "walkthrough ingestion completed",
		"game_id", request.GameID, "document_id", result.DocumentID,
		"chunks", result.ChunkCount, "duration_ms", time.Since(started).Milliseconds())
	return result, nil
}
