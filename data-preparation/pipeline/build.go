package pipeline

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/walkthrough-assistance/data-preparation/model"
	writer "github.com/walkthrough-assistance/data-preparation/rag"
)

func Build(ctx context.Context, c Config) error {
	a, e := Load(c.Artifacts)
	if e != nil {
		return e
	}
	if e = validate(a, c.RequireReview); e != nil {
		return e
	}
	if a.Manifest.GameID != c.GameID || a.Manifest.GameName != c.GameName {
		return fmt.Errorf("config game differs from artifact manifest")
	}
	p, ok := c.Providers[c.EmbeddingProvider]
	if !ok {
		return fmt.Errorf("embedding_provider must name a configured provider")
	}
	if strings.ToLower(filepath.Ext(c.Output)) != ".rag" {
		return fmt.Errorf("output must have .rag extension")
	}
	if _, e = os.Stat(c.Output); !os.IsNotExist(e) {
		return fmt.Errorf("output already exists or is inaccessible: %s", c.Output)
	}
	doc := model.Document{GameID: c.GameID, GameName: c.GameName, SourceURL: "file:" + a.Manifest.InputName, Title: c.GameName, RawHTML: a.Raw, Markdown: a.Document, ParserMetadata: map[string]string{"preparation_version": Version, "source_sha256": a.Manifest.RawSHA256}, ChunkingVersion: Version, Embedding: model.EmbeddingInfo{Model: p.Model, Version: p.Version, Dimensions: p.Dimensions}}
	for start := 0; start < len(a.Entries); {
		end := min(start+16, len(a.Entries))
		texts := []string{}
		for _, entry := range a.Entries[start:end] {
			texts = append(texts, entry.SearchText)
		}
		vectors, e := embed(ctx, p, texts)
		if e != nil {
			return e
		}
		doc.Embeddings = append(doc.Embeddings, vectors...)
		start = end
	}
	for i, entry := range a.Entries {
		doc.Chunks = append(doc.Chunks, model.Chunk{Title: c.GameName, Section: entry.Section, Sequence: i, Text: entry.SearchText, Metadata: map[string]any{"preparation_card": a.Cards[i], "situation": map[string]any{"schema": "situation-v1", "situation_id": entry.SituationID, "answer_text": entry.AnswerText, "source_spans": entry.SourceSpans}}})
	}
	parent := filepath.Dir(c.Output)
	if e = os.MkdirAll(parent, 0755); e != nil {
		return e
	}
	tmp, e := os.MkdirTemp(parent, ".rag-build-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(tmp)
	repo, e := writer.NewRepository(tmp)
	if e != nil {
		return e
	}
	result, e := repo.Save(ctx, doc)
	if e != nil {
		return e
	}
	db, e := sql.Open("sqlite", result.RAGPath)
	if e != nil {
		return e
	}
	manifestJSON, e := json.Marshal(a.Manifest)
	if e != nil {
		db.Close()
		return e
	}
	metadata := map[string]string{"preparation_manifest": string(manifestJSON), "source_sha256": a.Manifest.RawSHA256, "document_sha256": a.Manifest.DocumentSHA256, "preparation_version": Version, "embedding_input_format": "raw-search-text-v1", "annotation_model": a.Manifest.AnnotationModel}
	for k, v := range metadata {
		if _, e = db.ExecContext(ctx, "INSERT INTO metadata(key,value) VALUES (?,?)", k, v); e != nil {
			db.Close()
			return e
		}
	}
	var check string
	e = db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&check)
	closeErr := db.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	if check != "ok" {
		return fmt.Errorf("SQLite integrity_check: %s", check)
	}
	// Hard link publishes a complete closed SQLite file without replacing an existing one.
	return os.Link(result.RAGPath, c.Output)
}
