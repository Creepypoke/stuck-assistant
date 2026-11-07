package query

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCompleteLimitsGenerationAndDisablesThinking(t *testing.T) {
	t.Parallel()
	var payload struct {
		MaxTokens          int             `json:"max_tokens"`
		ChatTemplateKwargs map[string]bool `json:"chat_template_kwargs"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"choices":[{"message":{"content":"Go through the door."}}]}`))
	}))
	defer server.Close()

	service := &Service{
		config: Config{LLMURL: server.URL, LLMModel: "assistance"},
		client: server.Client(),
	}
	answer, err := service.complete(context.Background(), Request{Question: "Where next?", AnswerLength: "short"}, "Guide text")
	if err != nil {
		t.Fatal(err)
	}
	if answer != "Go through the door." {
		t.Fatalf("unexpected answer: %q", answer)
	}
	if payload.MaxTokens != 256 || payload.ChatTemplateKwargs["enable_thinking"] {
		t.Fatalf("unexpected generation options: %#v", payload)
	}
}

func TestThinkingOverridesPreserveReasoningAndTruncatedAttempts(t *testing.T) {
	var budgets []int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			MaxTokens   int             `json:"max_tokens"`
			Temperature float64         `json:"temperature"`
			Kwargs      map[string]bool `json:"chat_template_kwargs"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.Temperature != 0.9 {
			t.Errorf("temperature override lost: %v", payload.Temperature)
		}
		if !payload.Kwargs["enable_thinking"] {
			t.Error("thinking override was lost")
		}
		budgets = append(budgets, payload.MaxTokens)
		if len(budgets) == 1 {
			_, _ = w.Write([]byte(`{"choices":[{"finish_reason":"length","message":{"content":"partial","reasoning_content":"reasoning attempt one"}}],"usage":{"prompt_tokens":20,"completion_tokens":64,"completion_tokens_details":{"reasoning_tokens":60}}}`))
		} else {
			_, _ = w.Write([]byte(`{"choices":[{"finish_reason":"stop","message":{"content":"0392","reasoning":"reasoning attempt two"}}],"usage":{"prompt_tokens":20,"completion_tokens":80}}`))
		}
	}))
	defer server.Close()
	temperature := 0.9
	s := &Service{config: Config{GenerationTemperature: &temperature, LLMURL: server.URL, LLMModel: "fixture", EnableThinking: true, GenerationMaxTokens: 64, GenerationRetryMaxTokens: 128}, client: server.Client()}
	var attempts []CompletionTrace
	ctx := context.WithValue(context.Background(), completionTraceKey{}, &attempts)
	answer, _, err := s.completeWithPayload(ctx, Request{AnswerLength: "short"}, "Guide")
	if err != nil || answer != "0392" || len(budgets) != 2 || budgets[0] != 64 || budgets[1] != 128 {
		t.Fatalf("answer=%q budgets=%v err=%v", answer, budgets, err)
	}
	if len(attempts) != 2 || attempts[0].Content != "partial" || attempts[0].ReasoningContent != "reasoning attempt one" || attempts[0].ReasoningTokens == nil || *attempts[0].ReasoningTokens != 60 || attempts[1].ReasoningContent != "reasoning attempt two" {
		t.Fatalf("lost reasoning/retry diagnostics: %+v", attempts)
	}
}

func TestCompleteSendsLLMAPIKeyOnlyAsHeader(t *testing.T) {
	t.Parallel()
	const key = "test-secret-not-in-payload"
	var receivedAuthorization, receivedBody string
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		receivedAuthorization = request.Header.Get("Authorization")
		var body json.RawMessage
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		receivedBody = string(body)
		_, _ = response.Write([]byte(`{"choices":[{"finish_reason":"stop","message":{"content":"Go left."}}]}`))
	}))
	defer server.Close()
	s := &Service{config: Config{LLMURL: server.URL, LLMModel: "assistance", LLMAPIKey: key}, client: server.Client()}
	answer, debugPayload, err := s.completeWithPayload(context.Background(), Request{Question: "Where next?"}, "Go left.")
	if err != nil || answer != "Go left." || receivedAuthorization != "Bearer "+key ||
		strings.Contains(receivedBody, key) || strings.Contains(string(debugPayload), key) {
		t.Fatalf("unexpected LLM request: answer=%q authorization=%q body=%q debug=%q err=%v", answer, receivedAuthorization, receivedBody, debugPayload, err)
	}
}

func TestCompleteWithoutLLMAPIKeyDoesNotSendAuthorization(t *testing.T) {
	t.Parallel()
	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		authorization = request.Header.Get("Authorization")
		_, _ = response.Write([]byte(`{"choices":[{"message":{"content":"Go left."}}]}`))
	}))
	defer server.Close()
	s := &Service{config: Config{LLMURL: server.URL, LLMModel: "assistance"}, client: server.Client()}
	if _, err := s.complete(context.Background(), Request{Question: "Where next?"}, "Go left."); err != nil {
		t.Fatal(err)
	}
	if authorization != "" {
		t.Fatalf("unexpected authorization header: %q", authorization)
	}
}

func TestCompleteRetriesTruncatedGenerationAndReturnsFinalPayload(t *testing.T) {
	t.Parallel()
	var budgets []int
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		var payload struct {
			MaxTokens int `json:"max_tokens"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Error(err)
			return
		}
		budgets = append(budgets, payload.MaxTokens)
		if len(budgets) == 1 {
			_, _ = response.Write([]byte(`{"choices":[{"finish_reason":"length","message":{"content":"Go to **"}}]}`))
		} else {
			_, _ = response.Write([]byte(`{"choices":[{"finish_reason":"stop","message":{"content":"Go to the storage."}}]}`))
		}
	}))
	defer server.Close()
	s := &Service{config: Config{LLMURL: server.URL, LLMModel: "assistance"}, client: server.Client()}
	answer, payload, err := s.completeWithPayload(context.Background(), Request{AnswerLength: "short"}, "Guide")
	if err != nil || answer != "Go to the storage." || len(budgets) != 2 || budgets[0] != 256 || budgets[1] != 512 || !strings.Contains(string(payload), `"max_tokens":512`) {
		t.Fatalf("answer=%q budgets=%v payload=%s err=%v", answer, budgets, payload, err)
	}
}

func TestCompleteRejectsRepeatedTruncation(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write([]byte(`{"choices":[{"finish_reason":"length","message":{"content":"Partial **"}}]}`))
	}))
	defer server.Close()
	s := &Service{config: Config{LLMURL: server.URL, LLMModel: "assistance"}, client: server.Client()}
	answer, err := s.complete(context.Background(), Request{AnswerLength: "short"}, "Guide")
	if answer != "" || err == nil || !strings.Contains(err.Error(), "truncated") {
		t.Fatalf("answer=%q err=%v", answer, err)
	}
}

func TestCompletionPayloadUsesDefaultGuideAndQuestion(t *testing.T) {
	t.Parallel()
	service := &Service{config: Config{LLMModel: "assistance"}}
	payload, err := service.completionPayload(Request{
		GameID: "dino-crisis-psx", Question: "Куда идти?", NoSpoilers: true, AnswerLength: "short",
	}, "Open the door.")
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Messages) != 2 || !strings.Contains(body.Messages[0].Content, "same language") ||
		!strings.Contains(body.Messages[0].Content, "Avoid spoilers") ||
		!strings.Contains(body.Messages[1].Content, "Game ID: dino-crisis-psx") ||
		!strings.Contains(body.Messages[1].Content, "Open the door.") {
		t.Fatalf("unexpected model messages: %+v", body.Messages)
	}
}

func TestCompletionPayloadUsesCustomPrompt(t *testing.T) {
	t.Parallel()
	service := &Service{config: Config{LLMModel: "assistance", SystemPrompt: "You are a cheerful guide.\nBe encouraging."}}
	payload, err := service.completionPayload(Request{Question: "Where next?", AnswerLength: "long"}, "Go left.")
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(body.Messages[0].Content, "You are a cheerful guide.\nBe encouraging.") ||
		strings.Contains(body.Messages[0].Content, "You are a friendly") ||
		!strings.Contains(body.Messages[0].Content, "Requested answer length: long") {
		t.Fatalf("unexpected custom system prompt: %q", body.Messages[0].Content)
	}
}

func TestAskNoRAGSkipsRetrievalAndSendsOnlyQuestion(t *testing.T) {
	t.Parallel()
	var payload struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		_, _ = response.Write([]byte(`{"choices":[{"finish_reason":"stop","message":{"content":"Try the entrance."}}]}`))
	}))
	defer server.Close()
	// Neither an embedding provider nor a repository is configured: a retrieval
	// attempt would fail, while no-RAG must only call the LLM.
	s := &Service{config: Config{LLMURL: server.URL, LLMModel: "assistance"}, client: server.Client()}
	debug, sources, answer, err := s.AskDebug(context.Background(), Request{
		GameID: "dino-crisis-psx", Question: "Куда идти?", NoRAG: true, NoSpoilers: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if answer != "Try the entrance." || len(sources) != 0 || debug.RetrievedChunks == nil || len(debug.RetrievedChunks) != 0 {
		t.Fatalf("unexpected no-RAG result: answer=%q sources=%#v debug=%#v", answer, sources, debug)
	}
	if len(payload.Messages) != 2 || !strings.Contains(payload.Messages[0].Content, "general knowledge") ||
		strings.Contains(payload.Messages[0].Content, "Ground every concrete direction") ||
		strings.Contains(payload.Messages[1].Content, "Walkthrough context:") ||
		!strings.Contains(payload.Messages[1].Content, "Question: Куда идти?") ||
		strings.Contains(string(debug.ModelRequest), `"no_rag"`) {
		// The LLM payload must contain only model settings, messages and the
		// question; the no-rag flag belongs to the service request, not LLM.
		t.Fatalf("unexpected no-RAG model request: %s", debug.ModelRequest)
	}
}
