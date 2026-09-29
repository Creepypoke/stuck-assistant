package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/walkthrough-assistance/backend/llama"
	"github.com/walkthrough-assistance/backend/query"
)

type recordingAsker struct {
	request query.Request
}

type debugRecordingAsker struct{ recordingAsker }

func (asker *debugRecordingAsker) AskDebug(_ context.Context, request query.Request) (query.DebugInfo, []query.Source, string, error) {
	asker.request = request
	return query.DebugInfo{
		Request:         request,
		RetrievedChunks: []query.RetrievedChunk{{Title: "Guide", Section: "First area", Text: "Open the door."}},
		ModelRequest:    []byte(`{"model":"assistance","messages":[{"role":"user","content":"Open the door."}]}`),
	}, []query.Source{{Title: "Guide"}}, "Go through the front door.", nil
}

func (asker *recordingAsker) Ask(_ context.Context, request query.Request) ([]query.Source, string, error) {
	asker.request = request
	return []query.Source{{Title: "Guide", Section: "First area", URL: "https://example.com"}}, "Go through the front door.", nil
}

func TestStatusEndpointReportsLlamaState(t *testing.T) {
	handler := newHandler(func(context.Context) llama.Status {
		return llama.Status{Running: true, Healthy: true}
	})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status code: %d", response.Code)
	}
	if got := response.Body.String(); got != "{\"service\":\"running\",\"llama\":{\"running\":true,\"healthy\":true}}\n" {
		t.Fatalf("unexpected response: %s", got)
	}
}

func TestHealthEndpointIsUnavailableUntilLlamaIsHealthy(t *testing.T) {
	handler := newHandler(func(context.Context) llama.Status {
		return llama.Status{Running: true, Error: "starting"}
	})
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("unexpected status code: %d", response.Code)
	}
}

func TestAskEndpointStreamsAnswer(t *testing.T) {
	asker := &recordingAsker{}
	handler := newHandler(func(context.Context) llama.Status { return llama.Status{} }, asker)
	request := httptest.NewRequest(http.MethodPost, "/ask", strings.NewReader(`{
        "game":{"gameId":"dino-crisis-psx"},"question":"How do I enter the first area?",
        "context":{"location":"lobby"},"options":{"noSpoilers":true,"answerLength":"short"}
    }`))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "event: token") ||
		!strings.Contains(response.Body.String(), "Go through the front door.") {
		t.Fatalf("unexpected SSE response: %d %q", response.Code, response.Body.String())
	}
	if asker.request.GameID != "dino-crisis-psx" || asker.request.Question != "How do I enter the first area?" {
		t.Fatalf("unexpected ask request: %#v", asker.request)
	}
}

func TestAskEndpointIncludesModelContextForDebugRequest(t *testing.T) {
	asker := &debugRecordingAsker{}
	handler := newHandler(func(context.Context) llama.Status { return llama.Status{} }, asker)
	request := httptest.NewRequest(http.MethodPost, "/ask", strings.NewReader(`{"game":{"gameId":"dino-crisis-psx"},"question":"Where next?","context":{},"options":{}}`))
	request.Header.Set("X-Walkthrough-Debug", "true")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if !strings.Contains(response.Body.String(), "event: debug") || !strings.Contains(response.Body.String(), "Open the door.") {
		t.Fatalf("debug context was not returned: %q", response.Body.String())
	}
}

func TestAskEndpointForwardsNoRAG(t *testing.T) {
	asker := &recordingAsker{}
	handler := newHandler(func(context.Context) llama.Status { return llama.Status{} }, asker)
	request := httptest.NewRequest(http.MethodPost, "/ask", strings.NewReader(`{"game":{"gameId":"dino-crisis-psx"},"question":"Where next?","options":{"noRag":true}}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if !asker.request.NoRAG || strings.Contains(response.Body.String(), "retrieval") || !strings.Contains(response.Body.String(), "event: source") {
		t.Fatalf("unexpected no-RAG response: request=%#v body=%q", asker.request, response.Body.String())
	}
}

func TestAskNoRAGWithoutInstalledKnowledge(t *testing.T) {
	model := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected request to model: %s", request.URL.Path)
			http.Error(response, "unexpected request", http.StatusBadRequest)
			return
		}
		_, _ = response.Write([]byte(`{"choices":[{"finish_reason":"stop","message":{"content":"Ask me about the game."}}]}`))
	}))
	defer model.Close()
	asker, err := query.New(query.Config{
		StorageDirectory: t.TempDir(), EmbeddingURL: model.URL,
		EmbeddingModel: "embedding", EmbeddingVersion: "1", EmbeddingDimensions: 2,
		LLMURL: model.URL, LLMModel: "assistance",
	})
	if err != nil {
		t.Fatal(err)
	}
	handler := newHandler(func(context.Context) llama.Status { return llama.Status{} }, asker)
	request := httptest.NewRequest(http.MethodPost, "/ask", strings.NewReader(`{"game":{"gameId":"dino-crisis-psx"},"question":"Where next?","options":{"noRag":true}}`))
	request.Header.Set("X-Walkthrough-Debug", "true")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	body := response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(body, `"retrieved_chunks":[]`) ||
		!strings.Contains(body, "Ask me about the game.") || strings.Contains(body, "Walkthrough context") ||
		strings.Contains(body, "event: source") || strings.Contains(body, "retrieval") {
		t.Fatalf("unexpected no-RAG SSE response: %d %q", response.Code, body)
	}
}
