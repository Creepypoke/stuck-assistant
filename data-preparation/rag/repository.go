package rag

import (
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"github.com/walkthrough-assistance/data-preparation/model"
	_ "modernc.org/sqlite"
)

const schemaVersion = "1"

var gameIDPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type Repository struct {
	root string
}

func NewRepository(root string) (*Repository, error) {
	if root == "" {
		return nil, errors.New("RAG storage directory is required")
	}
	return &Repository{root: root}, nil
}

func (r *Repository) Save(ctx context.Context, document model.Document) (model.Result, error) {
	if !gameIDPattern.MatchString(document.GameID) {
		return model.Result{}, fmt.Errorf("invalid game ID %q: expected a stable lowercase slug", document.GameID)
	}
	if len(document.Chunks) == 0 || len(document.Chunks) != len(document.Embeddings) {
		return model.Result{}, fmt.Errorf("chunks and embeddings must be non-empty and have equal lengths")
	}
	if document.Embedding.Dimensions <= 0 || document.Embedding.Model == "" {
		return model.Result{}, fmt.Errorf("embedding metadata is incomplete")
	}
	for index, vector := range document.Embeddings {
		if len(vector) != document.Embedding.Dimensions {
			return model.Result{}, fmt.Errorf("embedding %d has %d dimensions, expected %d", index, len(vector), document.Embedding.Dimensions)
		}
	}

	if err := os.MkdirAll(r.root, 0o755); err != nil {
		return model.Result{}, fmt.Errorf("create RAG storage directory: %w", err)
	}
	path := filepath.Join(r.root, document.GameID+".rag")
	database, err := sql.Open("sqlite", path)
	if err != nil {
		return model.Result{}, fmt.Errorf("open RAG file: %w", err)
	}
	defer database.Close()
	database.SetMaxOpenConns(1)

	if _, err := database.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		return model.Result{}, fmt.Errorf("enable foreign keys: %w", err)
	}
	if err := initialize(ctx, database); err != nil {
		return model.Result{}, err
	}

	transaction, err := database.BeginTx(ctx, nil)
	if err != nil {
		return model.Result{}, fmt.Errorf("begin ingestion transaction: %w", err)
	}
	defer transaction.Rollback()

	if err := validateAndUpdateMetadata(ctx, transaction, document); err != nil {
		return model.Result{}, err
	}
	parserMetadata, err := json.Marshal(document.ParserMetadata)
	if err != nil {
		return model.Result{}, fmt.Errorf("encode parser metadata: %w", err)
	}
	downloadedAt := document.DownloadedAt
	if downloadedAt.IsZero() {
		downloadedAt = time.Now().UTC()
	}
	result, err := transaction.ExecContext(ctx, `
		INSERT INTO documents(source_url, title, raw_html, markdown, parser_metadata, downloaded_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		document.SourceURL, document.Title, document.RawHTML, document.Markdown, parserMetadata,
		downloadedAt.Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return model.Result{}, fmt.Errorf("insert document: %w", err)
	}
	documentID, err := result.LastInsertId()
	if err != nil {
		return model.Result{}, fmt.Errorf("read document ID: %w", err)
	}

	for index, chunk := range document.Chunks {
		metadata, err := json.Marshal(chunk.Metadata)
		if err != nil {
			return model.Result{}, fmt.Errorf("encode chunk %d metadata: %w", index, err)
		}
		chunkResult, err := transaction.ExecContext(ctx, `
			INSERT INTO chunks(document_id, title, section, sequence, text, metadata)
			VALUES (?, ?, ?, ?, ?, ?)`,
			documentID, chunk.Title, chunk.Section, chunk.Sequence, chunk.Text, metadata,
		)
		if err != nil {
			return model.Result{}, fmt.Errorf("insert chunk %d: %w", index, err)
		}
		chunkID, err := chunkResult.LastInsertId()
		if err != nil {
			return model.Result{}, fmt.Errorf("read chunk %d ID: %w", index, err)
		}
		if _, err := transaction.ExecContext(ctx,
			"INSERT INTO embeddings(chunk_id, dimensions, vector) VALUES (?, ?, ?)",
			chunkID, document.Embedding.Dimensions, encodeVector(document.Embeddings[index])); err != nil {
			return model.Result{}, fmt.Errorf("insert embedding %d: %w", index, err)
		}
	}

	if err := transaction.Commit(); err != nil {
		return model.Result{}, fmt.Errorf("commit ingestion transaction: %w", err)
	}
	return model.Result{DocumentID: documentID, ChunkCount: len(document.Chunks), RAGPath: path}, nil
}

func initialize(ctx context.Context, database *sql.DB) error {
	const schema = `
		CREATE TABLE IF NOT EXISTS metadata (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS documents (
			id INTEGER PRIMARY KEY,
			source_url TEXT NOT NULL,
			title TEXT NOT NULL,
			raw_html BLOB,
			markdown TEXT NOT NULL,
			parser_metadata TEXT NOT NULL DEFAULT '{}',
			downloaded_at TEXT NOT NULL,
			created_at TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS chunks (
			id INTEGER PRIMARY KEY,
			document_id INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
			title TEXT NOT NULL,
			section TEXT NOT NULL DEFAULT '',
			sequence INTEGER NOT NULL,
			text TEXT NOT NULL,
			metadata TEXT NOT NULL DEFAULT '{}',
			UNIQUE(document_id, sequence)
		);
		CREATE TABLE IF NOT EXISTS embeddings (
			chunk_id INTEGER PRIMARY KEY REFERENCES chunks(id) ON DELETE CASCADE,
			dimensions INTEGER NOT NULL,
			vector BLOB NOT NULL
		);
		CREATE VIRTUAL TABLE IF NOT EXISTS chunks_fts USING fts5(
			title, section, text, content='chunks', content_rowid='id'
		);
		CREATE TRIGGER IF NOT EXISTS chunks_after_insert AFTER INSERT ON chunks BEGIN
			INSERT INTO chunks_fts(rowid, title, section, text)
			VALUES (new.id, new.title, new.section, new.text);
		END;
		CREATE TRIGGER IF NOT EXISTS chunks_after_delete AFTER DELETE ON chunks BEGIN
			INSERT INTO chunks_fts(chunks_fts, rowid, title, section, text)
			VALUES ('delete', old.id, old.title, old.section, old.text);
		END;
		CREATE TRIGGER IF NOT EXISTS chunks_after_update AFTER UPDATE ON chunks BEGIN
			INSERT INTO chunks_fts(chunks_fts, rowid, title, section, text)
			VALUES ('delete', old.id, old.title, old.section, old.text);
			INSERT INTO chunks_fts(rowid, title, section, text)
			VALUES (new.id, new.title, new.section, new.text);
		END;`
	if _, err := database.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("initialize RAG schema: %w", err)
	}
	return nil
}

func validateAndUpdateMetadata(ctx context.Context, transaction *sql.Tx, document model.Document) error {
	expected := map[string]string{
		"schema_version":       schemaVersion,
		"game_id":              document.GameID,
		"embedding_model":      document.Embedding.Model,
		"embedding_dimensions": strconv.Itoa(document.Embedding.Dimensions),
		"embedding_version":    document.Embedding.Version,
		"chunking_version":     document.ChunkingVersion,
	}
	for key, value := range expected {
		var existing string
		err := transaction.QueryRowContext(ctx, "SELECT value FROM metadata WHERE key = ?", key).Scan(&existing)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("read RAG metadata %q: %w", key, err)
		}
		if err == nil && existing != value {
			return fmt.Errorf("incompatible RAG metadata %q: file has %q, ingestion requires %q", key, existing, value)
		}
		if _, err := transaction.ExecContext(ctx,
			"INSERT INTO metadata(key, value) VALUES (?, ?) ON CONFLICT(key) DO NOTHING", key, value); err != nil {
			return fmt.Errorf("write RAG metadata %q: %w", key, err)
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	mutable := map[string]string{
		"game_name":      document.GameName,
		"updated_at":     now,
		"source_url":     document.SourceURL,
		"source_version": downloadedVersion(document.DownloadedAt),
	}
	for key, value := range mutable {
		if _, err := transaction.ExecContext(ctx, `
			INSERT INTO metadata(key, value) VALUES (?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value); err != nil {
			return fmt.Errorf("update RAG metadata %q: %w", key, err)
		}
	}
	if _, err := transaction.ExecContext(ctx,
		"INSERT INTO metadata(key, value) VALUES ('created_at', ?) ON CONFLICT(key) DO NOTHING", now); err != nil {
		return fmt.Errorf("write RAG creation time: %w", err)
	}
	return nil
}

func downloadedVersion(value time.Time) string {
	if value.IsZero() {
		return time.Now().UTC().Format(time.RFC3339Nano)
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func encodeVector(vector []float32) []byte {
	encoded := make([]byte, len(vector)*4)
	for index, value := range vector {
		binary.LittleEndian.PutUint32(encoded[index*4:], math.Float32bits(value))
	}
	return encoded
}
