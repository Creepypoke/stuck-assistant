package downloader

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/walkthrough-assistance/backend/ingestion"
)

func TestDownloadFollowsRedirectAndKeepsFinalURL(t *testing.T) {
	t.Parallel()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/start" {
			http.Redirect(response, request, server.URL+"/guide", http.StatusFound)
			return
		}
		if request.Header.Get("User-Agent") != "test-agent" {
			t.Errorf("unexpected User-Agent: %q", request.Header.Get("User-Agent"))
		}
		response.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = response.Write([]byte("<article>guide</article>"))
	}))
	defer server.Close()

	document, err := New(Config{UserAgent: "test-agent"}).Download(context.Background(), ingestion.Source{URL: server.URL + "/start"})
	if err != nil {
		t.Fatal(err)
	}
	if document.URL != server.URL+"/guide" {
		t.Fatalf("final URL = %q", document.URL)
	}
	if string(document.Content) != "<article>guide</article>" {
		t.Fatalf("content = %q", document.Content)
	}
}

func TestDownloadRejectsOversizedDocument(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "text/html")
		_, _ = response.Write([]byte(strings.Repeat("x", 11)))
	}))
	defer server.Close()

	_, err := New(Config{MaxSize: 10}).Download(context.Background(), ingestion.Source{URL: server.URL})
	if err == nil || !strings.Contains(err.Error(), "exceeds 10 bytes") {
		t.Fatalf("expected size error, got %v", err)
	}
}

func TestDownloadValidatesSourceURL(t *testing.T) {
	t.Parallel()
	_, err := New(Config{}).Download(context.Background(), ingestion.Source{URL: "file:///tmp/guide"})
	if !errors.Is(err, ingestion.ErrInvalidSource) {
		t.Fatalf("expected ErrInvalidSource, got %v", err)
	}
}
