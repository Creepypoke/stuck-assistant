package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/walkthrough-assistance/backend/llama"
	"github.com/walkthrough-assistance/backend/logging"
	"github.com/walkthrough-assistance/backend/query"
	"github.com/walkthrough-assistance/backend/service"
)

func main() {
	loadEnvironment()
	logger, err := logging.New(os.Stderr, os.Getenv("WALKTHROUGH_LOG_LEVEL"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	preset := environmentPathOrDefault("WALKTHROUGH_LLAMA_MODELS_PRESET", defaultPresetPath())
	queryConfig, err := queryConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	config := service.Config{
		Address: environmentOrDefault("WALKTHROUGH_LISTEN_ADDRESS", "127.0.0.1:19273"),
		Query:   queryConfig,
		Llama: llama.Config{
			Command: environmentOrDefault("WALKTHROUGH_LLAMA_COMMAND", "llama"),
			Args: []string{
				"serve", "--host", "127.0.0.1", "--port", "8081",
				"--models-preset", preset, "--models-max", "2",
			},
			HealthURL:     environmentOrDefault("WALKTHROUGH_LLAMA_HEALTH_URL", "http://127.0.0.1:8081/health"),
			HealthTimeout: 2 * time.Second,
		},
	}
	if arguments := strings.Fields(os.Getenv("WALKTHROUGH_LLAMA_ARGS")); len(arguments) > 0 {
		config.Llama.Args = arguments
	}

	if err := service.Run(ctx, config, logger); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func queryConfig() (query.Config, error) {
	timeout, err := environmentDuration("WALKTHROUGH_LLM_TIMEOUT", 10*time.Minute)
	if err != nil {
		return query.Config{}, err
	}
	var systemPrompt string
	if path := environmentPathOrDefault("WALKTHROUGH_SYSTEM_PROMPT_FILE", ""); path != "" {
		contents, err := os.ReadFile(path)
		if err != nil {
			return query.Config{}, fmt.Errorf("read system prompt file %q: %w", path, err)
		}
		if strings.TrimSpace(string(contents)) == "" {
			return query.Config{}, fmt.Errorf("system prompt file %q is empty", path)
		}
		systemPrompt = string(contents)
	}
	cutoff := 0.0
	if value := os.Getenv("WALKTHROUGH_RELATIVE_SCORE_CUTOFF"); value != "" {
		cutoff, err = strconv.ParseFloat(value, 64)
		if err != nil || cutoff < 0 || cutoff > 1 {
			return query.Config{}, fmt.Errorf("parse WALKTHROUGH_RELATIVE_SCORE_CUTOFF: expected a number from 0 to 1")
		}
	}
	return query.Config{
		StorageDirectory:    storageDirectory(),
		EmbeddingURL:        environmentOrDefault("WALKTHROUGH_EMBEDDING_URL", "http://127.0.0.1:8081"),
		EmbeddingModel:      environmentOrDefault("WALKTHROUGH_EMBEDDING_MODEL", "embedding"),
		EmbeddingVersion:    environmentOrDefault("WALKTHROUGH_EMBEDDING_VERSION", "1"),
		EmbeddingDimensions: 768,
		LLMURL:              environmentOrDefault("WALKTHROUGH_LLM_URL", "http://127.0.0.1:8081"),
		LLMModel:            environmentOrDefault("WALKTHROUGH_LLM_MODEL", "assistance"),
		LLMAPIKey:           os.Getenv("WALKTHROUGH_LLM_API_KEY"),
		SystemPrompt:        systemPrompt,
		Timeout:             timeout,
		ResultLimit:         5,
		RetrievalMode:       environmentOrDefault("WALKTHROUGH_RETRIEVAL_MODE", "hybrid"),
		PromptProfile:       environmentOrDefault("WALKTHROUGH_PROMPT_PROFILE", "default"),
		RelativeScoreCutoff: cutoff,
		CodeAnswers:         os.Getenv("WALKTHROUGH_CODE_ANSWERS") == "1",
	}, nil
}

func storageDirectory() string {
	return environmentPathOrDefault("WALKTHROUGH_STORAGE_DIR", defaultStorageDirectory())
}

func environmentPathOrDefault(key, fallback string) string {
	directory := environmentOrDefault(key, fallback)
	if directory == "" || filepath.IsAbs(directory) {
		return directory
	}
	executable, err := os.Executable()
	if err != nil {
		return directory
	}
	return filepath.Join(filepath.Dir(executable), directory)
}

func defaultStorageDirectory() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join("data", "games")
	}
	return filepath.Join(home, ".local", "share", "walkthrough-assistance", "games")
}

func loadEnvironment() {
	executable, err := os.Executable()
	if err != nil {
		return
	}
	_ = godotenv.Load(filepath.Join(filepath.Dir(executable), ".env"))
}

func defaultPresetPath() string {
	executable, err := os.Executable()
	if err != nil {
		return filepath.Join("llama-cpp", "models.ini")
	}
	return filepath.Join(filepath.Dir(executable), "..", "llama-cpp", "models.ini")
}

func environmentOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func environmentDuration(key string, fallback time.Duration) (time.Duration, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		if err == nil {
			err = fmt.Errorf("must be positive")
		}
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	return duration, nil
}
