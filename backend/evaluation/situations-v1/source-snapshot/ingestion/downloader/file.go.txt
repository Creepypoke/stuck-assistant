package downloader

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/walkthrough-assistance/backend/ingestion"
)

// LocalFile reads supported walkthrough source files without making a network request.
type LocalFile struct {
	maxSize int64
}

func NewLocalFile(config Config) *LocalFile {
	maxSize := config.MaxSize
	if maxSize <= 0 {
		maxSize = defaultMaxSize
	}
	return &LocalFile{maxSize: maxSize}
}

func (d *LocalFile) Download(ctx context.Context, source ingestion.Source) (ingestion.RawDocument, error) {
	if err := ctx.Err(); err != nil {
		return ingestion.RawDocument{}, err
	}

	path, err := filepath.Abs(source.URL)
	if err != nil {
		return ingestion.RawDocument{}, fmt.Errorf("resolve file path: %w", err)
	}
	contentType, err := localContentType(path)
	if err != nil {
		return ingestion.RawDocument{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return ingestion.RawDocument{}, fmt.Errorf("read local file %q: %w", source.URL, err)
	}
	if !info.Mode().IsRegular() {
		return ingestion.RawDocument{}, fmt.Errorf("read local file %q: not a regular file", source.URL)
	}
	if info.Size() > d.maxSize {
		return ingestion.RawDocument{}, fmt.Errorf("read local file %q: document exceeds %d bytes", source.URL, d.maxSize)
	}

	file, err := os.Open(path)
	if err != nil {
		return ingestion.RawDocument{}, fmt.Errorf("open local file %q: %w", source.URL, err)
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, d.maxSize+1))
	if err != nil {
		return ingestion.RawDocument{}, fmt.Errorf("read local file %q: %w", source.URL, err)
	}
	if int64(len(content)) > d.maxSize {
		return ingestion.RawDocument{}, fmt.Errorf("read local file %q: document exceeds %d bytes", source.URL, d.maxSize)
	}
	if len(content) == 0 {
		return ingestion.RawDocument{}, errors.Join(ingestion.ErrEmptyDocument, fmt.Errorf("local file %q is empty", source.URL))
	}

	return ingestion.RawDocument{
		URL:          path,
		ContentType:  contentType,
		Content:      content,
		DownloadedAt: time.Now().UTC(),
	}, nil
}

func localContentType(path string) (string, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".html", ".htm":
		return "text/html", nil
	case ".txt":
		return "text/plain", nil
	default:
		return "", fmt.Errorf("%w: local file must have a .html, .htm, or .txt extension", ingestion.ErrInvalidSource)
	}
}
