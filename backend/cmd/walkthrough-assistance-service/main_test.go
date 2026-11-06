package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestQueryConfigReadsSystemPromptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prompt.txt")
	if err := os.WriteFile(path, []byte("Helpful guide\nSpeak naturally."), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WALKTHROUGH_SYSTEM_PROMPT_FILE", path)
	config, err := queryConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.SystemPrompt != "Helpful guide\nSpeak naturally." {
		t.Fatalf("unexpected prompt: %q", config.SystemPrompt)
	}
}

func TestQueryConfigRejectsEmptySystemPromptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prompt.txt")
	if err := os.WriteFile(path, []byte(" \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WALKTHROUGH_SYSTEM_PROMPT_FILE", path)
	if _, err := queryConfig(); err == nil {
		t.Fatal("expected error for empty prompt")
	}
}

func TestQueryConfigReadsRemoteLLMSettings(t *testing.T) {
	t.Setenv("WALKTHROUGH_LLM_URL", "https://openai.home-router.crazedns.ru")
	t.Setenv("WALKTHROUGH_LLM_MODEL", "remote-model")
	t.Setenv("WALKTHROUGH_LLM_API_KEY", "test-secret")
	t.Setenv("WALKTHROUGH_EMBEDDING_URL", "http://127.0.0.1:8081")
	config, err := queryConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.LLMURL != "https://openai.home-router.crazedns.ru" || config.LLMModel != "remote-model" ||
		config.LLMAPIKey != "test-secret" || config.EmbeddingURL != "http://127.0.0.1:8081" {
		t.Fatalf("unexpected remote LLM configuration: %#v", config)
	}
}

func TestEnvironmentPathOrDefaultResolvesRelativePathFromExecutable(t *testing.T) {
	t.Setenv("WALKTHROUGH_TEST_PATH", filepath.Join("data", "games"))
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(filepath.Dir(executable), "data", "games")
	if got := environmentPathOrDefault("WALKTHROUGH_TEST_PATH", ""); got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}

func TestEnvironmentPathOrDefaultPreservesAbsolutePath(t *testing.T) {
	t.Setenv("WALKTHROUGH_TEST_PATH", "/tmp/games")
	if got := environmentPathOrDefault("WALKTHROUGH_TEST_PATH", ""); got != "/tmp/games" {
		t.Fatalf("path = %q, want /tmp/games", got)
	}
}
