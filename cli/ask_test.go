package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAskSendsDeckyCompatibleRequest(t *testing.T) {
	t.Parallel()
	var received deckyAskRequest
	var raw map[string]json.RawMessage
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/ask" {
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("unexpected content type: %q", request.Header.Get("Content-Type"))
		}
		payload, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(payload, &raw); err != nil {
			t.Fatal(err)
		}
		if _, ok := raw["context"]; ok {
			t.Fatal("unexpected progress metadata in request")
		}
		if err := json.Unmarshal(payload, &received); err != nil {
			t.Fatal(err)
		}
		_, _ = response.Write([]byte("event: done\ndata: {}\n\n"))
	}))
	defer server.Close()

	command := newAskCommandWithClient(server.Client())
	output := new(bytes.Buffer)
	command.SetOut(output)
	command.SetArgs([]string{
		"dino-crisis-psx", "Where do I find the key?", "--service-url", server.URL,
		"--display-name", "Dino Crisis",
		"--no-spoilers=false", "--answer-length", "long",
	})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if received.Game != (deckyGame{Source: "local", GameID: "dino-crisis-psx", DisplayName: "Dino Crisis"}) {
		t.Fatalf("unexpected game: %#v", received.Game)
	}
	if received.Question != "Where do I find the key?" {
		t.Fatalf("unexpected question: %#v", received)
	}
	if received.Options != (deckyOptions{NoSpoilers: false, AnswerLength: "long"}) {
		t.Fatalf("unexpected options: %#v", received.Options)
	}
	if output.String() != "event: done\ndata: {}\n\n" {
		t.Fatalf("unexpected output: %q", output.String())
	}
}

func TestAskRequiresGameIDAndQuestion(t *testing.T) {
	t.Parallel()
	command := newAskCommand()
	command.SetArgs([]string{"dino-crisis-psx"})

	if err := command.Execute(); err == nil {
		t.Fatal("expected argument validation error")
	}
}

func TestAskSendsDebugHeaderAtDebugLogLevel(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("X-Walkthrough-Debug") != "true" {
			t.Fatalf("debug header was not sent")
		}
		_, _ = response.Write([]byte("event: done\ndata: {}\n\n"))
	}))
	defer server.Close()

	command := NewCommand()
	command.SetOut(new(bytes.Buffer))
	command.SetArgs([]string{"--log-level", "debug", "ask", "test-game", "Where next?", "--service-url", server.URL})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
}

func TestAskNoRAGFlag(t *testing.T) {
	t.Parallel()
	var received deckyAskRequest
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if err := json.NewDecoder(request.Body).Decode(&received); err != nil {
			t.Error(err)
		}
		_, _ = response.Write([]byte("event: done\ndata: {}\n\n"))
	}))
	defer server.Close()
	command := newAskCommandWithClient(server.Client())
	command.SetOut(new(bytes.Buffer))
	command.SetArgs([]string{"dino-crisis-psx", "Where next?", "--no-rag", "--service-url", server.URL})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if !received.Options.NoRAG {
		t.Fatalf("no-rag was not sent: %#v", received)
	}
}
