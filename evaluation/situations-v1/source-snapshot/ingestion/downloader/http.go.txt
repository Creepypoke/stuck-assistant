package downloader

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/walkthrough-assistance/backend/ingestion"
)

const (
	defaultTimeout   = 30 * time.Second
	defaultMaxSize   = int64(16 << 20)
	defaultUserAgent = "walkthrough-assistance/1.0"
)

type HTTPDownloader struct {
	client    *http.Client
	maxSize   int64
	userAgent string
}

type Config struct {
	Client    *http.Client
	Timeout   time.Duration
	MaxSize   int64
	UserAgent string
}

func New(config Config) *HTTPDownloader {
	client := config.Client
	if client == nil {
		timeout := config.Timeout
		if timeout <= 0 {
			timeout = defaultTimeout
		}
		client = &http.Client{Timeout: timeout}
	}
	maxSize := config.MaxSize
	if maxSize <= 0 {
		maxSize = defaultMaxSize
	}
	userAgent := config.UserAgent
	if userAgent == "" {
		userAgent = defaultUserAgent
	}
	return &HTTPDownloader{client: client, maxSize: maxSize, userAgent: userAgent}
}

func (d *HTTPDownloader) Download(ctx context.Context, source ingestion.Source) (ingestion.RawDocument, error) {
	parsedURL, err := url.ParseRequestURI(source.URL)
	if err != nil || parsedURL.Host == "" || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return ingestion.RawDocument{}, fmt.Errorf("%w: URL must use http or https", ingestion.ErrInvalidSource)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsedURL.String(), nil)
	if err != nil {
		return ingestion.RawDocument{}, fmt.Errorf("create request: %w", err)
	}
	request.Header.Set("User-Agent", d.userAgent)
	request.Header.Set("Accept", "text/html, text/plain;q=0.9")

	response, err := d.client.Do(request)
	if err != nil {
		return ingestion.RawDocument{}, fmt.Errorf("download %q: %w", source.URL, err)
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return ingestion.RawDocument{}, fmt.Errorf("download %q: unexpected HTTP status %s", source.URL, response.Status)
	}

	contentType := strings.ToLower(response.Header.Get("Content-Type"))
	if contentType != "" && !strings.HasPrefix(contentType, "text/html") && !strings.HasPrefix(contentType, "text/plain") {
		return ingestion.RawDocument{}, fmt.Errorf("download %q: unsupported content type %q", source.URL, contentType)
	}
	if response.ContentLength > d.maxSize {
		return ingestion.RawDocument{}, fmt.Errorf("download %q: document exceeds %d bytes", source.URL, d.maxSize)
	}

	content, err := io.ReadAll(io.LimitReader(response.Body, d.maxSize+1))
	if err != nil {
		return ingestion.RawDocument{}, fmt.Errorf("read response: %w", err)
	}
	if int64(len(content)) > d.maxSize {
		return ingestion.RawDocument{}, fmt.Errorf("download %q: document exceeds %d bytes", source.URL, d.maxSize)
	}
	if len(content) == 0 {
		return ingestion.RawDocument{}, errors.Join(ingestion.ErrEmptyDocument, errors.New("server returned an empty response"))
	}

	return ingestion.RawDocument{
		URL:          response.Request.URL.String(),
		ContentType:  response.Header.Get("Content-Type"),
		Content:      content,
		DownloadedAt: time.Now().UTC(),
	}, nil
}
