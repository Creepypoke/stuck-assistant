package downloader

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/walkthrough-assistance/backend/ingestion"
)

func TestLocalFileReadsSupportedFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "guide.txt")
	if err := os.WriteFile(path, []byte("Open the gate."), 0o600); err != nil {
		t.Fatal(err)
	}

	document, err := NewLocalFile(Config{}).Download(context.Background(), ingestion.Source{URL: path})
	if err != nil {
		t.Fatal(err)
	}
	if document.URL != path || document.ContentType != "text/plain" || string(document.Content) != "Open the gate." {
		t.Fatalf("unexpected local document: %#v", document)
	}
}

func TestLocalFileRejectsUnsupportedExtension(t *testing.T) {
	t.Parallel()
	_, err := NewLocalFile(Config{}).Download(context.Background(), ingestion.Source{URL: "guide.pdf"})
	if !errors.Is(err, ingestion.ErrInvalidSource) {
		t.Fatalf("expected ErrInvalidSource, got %v", err)
	}
}
