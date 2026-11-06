package model

import "time"

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
