package ingestion

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	ErrInvalidSource = errors.New("invalid walkthrough source")
	ErrEmptyDocument = errors.New("walkthrough document is empty")
	ErrNoChunks      = errors.New("walkthrough produced no chunks")
)

type Source struct {
	URL string
}

type RawDocument struct {
	URL          string
	ContentType  string
	Content      []byte
	DownloadedAt time.Time
}

type ParsedDocument struct {
	Title    string            `json:"title"`
	Markdown string            `json:"markdown"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type Chunk struct {
	Title    string
	Section  string
	Sequence int
	Text     string
	Metadata map[string]any
}

type EmbeddingInfo struct {
	Model      string
	Dimensions int
	Version    string
}

type Document struct {
	GameID          string
	GameName        string
	SourceURL       string
	Title           string
	RawHTML         []byte
	Markdown        string
	ParserMetadata  map[string]string
	Chunks          []Chunk
	Embeddings      [][]float32
	Embedding       EmbeddingInfo
	ChunkingVersion string
	DownloadedAt    time.Time
}

type Result struct {
	DocumentID int64
	ChunkCount int
	RAGPath    string
}

type WalkthroughProvider interface {
	Download(context.Context, Source) (RawDocument, error)
}

type DocumentParser interface {
	Parse(context.Context, RawDocument) (ParsedDocument, error)
}

type Chunker interface {
	Chunk(ParsedDocument) ([]Chunk, error)
	Version() string
}

type EmbeddingProvider interface {
	EmbedBatch(context.Context, []string) ([][]float32, error)
	Info() EmbeddingInfo
}

type Repository interface {
	Save(context.Context, Document) (Result, error)
}

type StageError struct {
	Stage string
	Err   error
}

func (e *StageError) Error() string {
	return fmt.Sprintf("ingestion %s failed: %v", e.Stage, e.Err)
}

func (e *StageError) Unwrap() error { return e.Err }
