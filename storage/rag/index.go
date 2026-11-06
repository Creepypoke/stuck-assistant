package rag

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
)

// index returns the validated chunks of a game, loading them on first use and
// again whenever the .rag file is replaced or modified.
func (r *Repository) index(ctx context.Context, gameID string) (*gameIndex, string, error) {
	path := filepath.Join(r.root, gameID+".rag")
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", fmt.Errorf("knowledge for game %q is not installed", gameID)
		}
		return nil, "", fmt.Errorf("stat RAG file: %w", err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if cached, ok := r.cache[path]; ok && sameFile(cached.file, info) {
		return cached, path, nil
	}
	delete(r.cache, path)
	chunks, err := loadChunks(ctx, path)
	if err != nil {
		return nil, "", err
	}
	index := &gameIndex{file: info, chunks: chunks}
	r.cache[path] = index
	return index, path, nil
}

func sameFile(a, b os.FileInfo) bool {
	return os.SameFile(a, b) && a.ModTime().Equal(b.ModTime()) && a.Size() == b.Size()
}

type documentEvidence struct {
	hash  string
	runes []rune
}

func openReadOnly(path string) (*sql.DB, error) {
	database, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("open RAG file: %w", err)
	}
	return database, nil
}

// loadChunks reads every chunk once, verifies its evidence against the stored
// document, and decodes its vector. Each document is hashed a single time.
func loadChunks(ctx context.Context, path string) ([]indexedChunk, error) {
	database, err := openReadOnly(path)
	if err != nil {
		return nil, err
	}
	defer database.Close()

	documents := map[int64]documentEvidence{}
	docRows, err := database.QueryContext(ctx, "SELECT id, markdown FROM documents")
	if err != nil {
		return nil, fmt.Errorf("read RAG documents: %w", err)
	}
	for docRows.Next() {
		var id int64
		var markdown string
		if err := docRows.Scan(&id, &markdown); err != nil {
			docRows.Close()
			return nil, fmt.Errorf("scan RAG document: %w", err)
		}
		documents[id] = documentEvidence{hash: DocumentHash(markdown), runes: []rune(markdown)}
	}
	if err := docRows.Err(); err != nil {
		docRows.Close()
		return nil, fmt.Errorf("iterate RAG documents: %w", err)
	}
	docRows.Close()

	rows, err := database.QueryContext(ctx, `
		SELECT c.id, c.title, c.section, c.text, d.source_url, e.dimensions, e.vector, c.document_id, c.sequence, c.metadata
		FROM embeddings e
		JOIN chunks c ON c.id = e.chunk_id
		JOIN documents d ON d.id = c.document_id`)
	if err != nil {
		return nil, fmt.Errorf("read RAG chunks: %w", err)
	}
	defer rows.Close()
	var chunks []indexedChunk
	for rows.Next() {
		var chunk indexedChunk
		var dimensions int
		var encoded, metadata []byte
		result := &chunk.result
		if err := rows.Scan(&result.ChunkID, &result.Title, &result.Section, &result.Text, &result.SourceURL, &dimensions, &encoded, &result.DocumentID, &result.Sequence, &metadata); err != nil {
			return nil, fmt.Errorf("scan RAG chunk: %w", err)
		}
		if err := ApplySituation(result, metadata); err != nil {
			return nil, err
		}
		document := documents[result.DocumentID]
		result.DocumentSHA256 = document.hash
		if err := validateSpans(*result, document.hash, document.runes); err != nil {
			return nil, err
		}
		if chunk.vector, err = decodeVector(encoded, dimensions); err != nil {
			return nil, err
		}
		chunks = append(chunks, chunk)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate RAG chunks: %w", err)
	}
	return chunks, nil
}
