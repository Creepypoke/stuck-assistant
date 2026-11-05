package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/walkthrough-assistance/backend/llama"
	"github.com/walkthrough-assistance/backend/query"
)

type Config struct {
	Address         string
	ShutdownTimeout time.Duration
	Llama           llama.Config
	Query           query.Config
}

type statusResponse struct {
	Service string       `json:"service"`
	Llama   llama.Status `json:"llama"`
}

type statusFunc func(context.Context) llama.Status

func Run(ctx context.Context, config Config, logger *slog.Logger) error {
	manager, err := llama.NewManager(config.Llama)
	if err != nil {
		return fmt.Errorf("configure llama.cpp: %w", err)
	}
	if err := manager.Start(ctx); err != nil {
		return err
	}
	asker, err := query.New(config.Query)
	if err != nil {
		return fmt.Errorf("configure question service: %w", err)
	}

	if config.Address == "" {
		config.Address = "127.0.0.1:19273"
	}
	if config.ShutdownTimeout <= 0 {
		config.ShutdownTimeout = 5 * time.Second
	}

	server := &http.Server{
		Addr:              config.Address,
		Handler:           newHandlerWithLogger(manager.Status, logger, asker),
		ReadHeaderTimeout: 5 * time.Second,
	}
	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("service API started", "address", config.Address)
		serverErrors <- server.ListenAndServe()
	}()

	shutdown := func() error {
		shutdownContext, cancel := context.WithTimeout(context.Background(), config.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownContext); err != nil {
			return fmt.Errorf("shutdown service API: %w", err)
		}
		return nil
	}

	select {
	case <-ctx.Done():
		return shutdown()
	case err := <-manager.Done():
		_ = shutdown()
		if err == nil {
			return errors.New("llama.cpp exited unexpectedly")
		}
		return fmt.Errorf("llama.cpp exited: %w", err)
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve API: %w", err)
	}
}

func newHandler(status statusFunc, askers ...query.Asker) http.Handler {
	return newHandlerWithLogger(status, slog.Default(), askers...)
}

func newHandlerWithLogger(status statusFunc, logger *slog.Logger, askers ...query.Asker) http.Handler {
	mux := http.NewServeMux()
	var asker query.Asker
	if len(askers) > 0 {
		asker = askers[0]
	}
	mux.HandleFunc("GET /health", func(response http.ResponseWriter, request *http.Request) {
		llamaStatus := status(request.Context())
		response.Header().Set("Content-Type", "application/json")
		response.Header().Set("Access-Control-Allow-Origin", "*")
		if !llamaStatus.Healthy {
			response.WriteHeader(http.StatusServiceUnavailable)
		}
		_ = json.NewEncoder(response).Encode(statusResponse{Service: "running", Llama: llamaStatus})
	})
	mux.HandleFunc("GET /api/v1/status", func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		response.Header().Set("Access-Control-Allow-Origin", "*")
		_ = json.NewEncoder(response).Encode(statusResponse{
			Service: "running",
			Llama:   status(request.Context()),
		})
	})
	mux.HandleFunc("POST /ask", func(response http.ResponseWriter, request *http.Request) {
		if asker == nil {
			http.Error(response, "question service is unavailable", http.StatusServiceUnavailable)
			return
		}
		var body struct {
			Game struct {
				GameID string `json:"gameId"`
			} `json:"game"`
			Question string `json:"question"`
			Options  struct {
				NoSpoilers   bool   `json:"noSpoilers"`
				AnswerLength string `json:"answerLength"`
				NoRAG        bool   `json:"noRag"`
			} `json:"options"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			http.Error(response, "invalid request body", http.StatusBadRequest)
			return
		}
		if body.Game.GameID == "" || strings.TrimSpace(body.Question) == "" {
			http.Error(response, "game.gameId and question are required", http.StatusBadRequest)
			return
		}
		response.Header().Set("Content-Type", "text/event-stream")
		response.Header().Set("Cache-Control", "no-cache")
		response.Header().Set("Access-Control-Allow-Origin", "*")
		if !body.Options.NoRAG {
			writeSSE(response, "status", map[string]string{"stage": "retrieval"})
		}
		if err := http.NewResponseController(response).Flush(); err != nil {
			return
		}
		queryRequest := query.Request{
			GameID: body.Game.GameID, Question: body.Question, NoSpoilers: body.Options.NoSpoilers,
			AnswerLength: body.Options.AnswerLength, NoRAG: body.Options.NoRAG,
		}
		var sources []query.Source
		var answer string
		var err error
		if request.Header.Get("X-Walkthrough-Debug") == "true" {
			if debugAsker, ok := asker.(query.DebugAsker); ok {
				debug, debugSources, debugAnswer, debugErr := debugAsker.AskDebug(request.Context(), queryRequest)
				sources, answer, err = debugSources, debugAnswer, debugErr
				if err == nil {
					logger.Debug("LLM context", "context", string(debug.ModelRequest), "retrieved_chunks", len(debug.RetrievedChunks))
					writeSSE(response, "debug", debug)
				}
			} else {
				sources, answer, err = asker.Ask(request.Context(), queryRequest)
			}
		} else {
			sources, answer, err = asker.Ask(request.Context(), queryRequest)
		}
		if err != nil {
			writeSSE(response, "error", map[string]string{"message": err.Error()})
			return
		}
		for _, source := range sources {
			writeSSE(response, "source", source)
		}
		writeSSE(response, "status", map[string]string{"stage": "generation"})
		writeSSE(response, "token", map[string]string{"text": answer})
		writeSSE(response, "done", map[string]bool{})
	})
	return mux
}

func writeSSE(response http.ResponseWriter, event string, value any) {
	payload, err := json.Marshal(value)
	if err == nil {
		_, _ = fmt.Fprintf(response, "event: %s\ndata: %s\n\n", event, payload)
	}
}
