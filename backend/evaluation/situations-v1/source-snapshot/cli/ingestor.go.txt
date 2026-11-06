package cli

import (
	"github.com/walkthrough-assistance/backend/embeddings/llamacpp"
	"github.com/walkthrough-assistance/backend/ingestion/chunker"
	"github.com/walkthrough-assistance/backend/ingestion/downloader"
	"github.com/walkthrough-assistance/backend/ingestion/parser"
	"github.com/walkthrough-assistance/backend/ingestion/providers"
	ingestionservice "github.com/walkthrough-assistance/backend/ingestion/service"
	"github.com/walkthrough-assistance/backend/storage/rag"
)

func NewIngestor(options Options) (Ingestor, error) {
	markdownChunker, err := chunker.New(chunker.Config{
		MaxTokens: options.ChunkTokens, OverlapTokens: options.OverlapTokens,
	})
	if err != nil {
		return nil, err
	}
	embeddings, err := llamacpp.New(llamacpp.Config{
		BaseURL: options.EmbeddingURL, Model: options.EmbeddingModel,
		Version: options.EmbeddingVersion, Dimensions: options.EmbeddingDimensions,
		Timeout: options.EmbeddingTimeout,
	})
	if err != nil {
		return nil, err
	}
	repository, err := rag.NewRepository(options.StorageDirectory)
	if err != nil {
		return nil, err
	}

	return ingestionservice.New(ingestionservice.Dependencies{
		Provider: providers.NewURLOrFile(
			downloader.New(downloader.Config{Timeout: options.DownloadTimeout}),
			downloader.NewLocalFile(downloader.Config{}),
		),
		Parser:  parser.NewCommand(options.ParserCommand, options.ParserArgs...),
		Chunker: markdownChunker, Embeddings: embeddings, Repository: repository,
	})
}
