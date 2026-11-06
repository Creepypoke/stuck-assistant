package rag

import (
	"errors"
	"os"
	"regexp"
	"sync"

	_ "modernc.org/sqlite"
)

var gameIDPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type Repository struct {
	root string

	// Validated chunks and decoded vectors per .rag path. An entry is reused
	// while the file on disk is unchanged, so a search does not re-read and
	// re-verify the whole database.
	mu    sync.Mutex
	cache map[string]*gameIndex
}

type gameIndex struct {
	file   os.FileInfo
	chunks []indexedChunk
}

type indexedChunk struct {
	result SearchResult
	vector []float32
}

func NewRepository(root string) (*Repository, error) {
	if root == "" {
		return nil, errors.New("RAG storage directory is required")
	}
	return &Repository{root: root, cache: map[string]*gameIndex{}}, nil
}
