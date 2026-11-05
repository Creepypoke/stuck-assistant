package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const defaultServiceURL = "http://127.0.0.1:19273"

type deckyAskRequest struct {
	Game     deckyGame    `json:"game"`
	Question string       `json:"question"`
	Options  deckyOptions `json:"options"`
}

type deckyGame struct {
	Source      string `json:"source"`
	GameID      string `json:"gameId"`
	DisplayName string `json:"displayName"`
}

type deckyOptions struct {
	NoSpoilers   bool   `json:"noSpoilers"`
	AnswerLength string `json:"answerLength"`
	NoRAG        bool   `json:"noRag,omitempty"`
}

func newAskCommand() *cobra.Command {
	return newAskCommandWithClient(&http.Client{})
}

func newAskCommandWithClient(client *http.Client) *cobra.Command {
	var serviceURL string
	var displayName string
	var noSpoilers bool
	var noRAG bool
	var answerLength string
	var timeout time.Duration

	command := &cobra.Command{
		Use:   "ask <game-id> <question>",
		Short: "Send a Decky-compatible question to the local service",
		Args:  cobra.ExactArgs(2),
		RunE: func(command *cobra.Command, args []string) error {
			logLevel := environmentOrDefault("WALKTHROUGH_LOG_LEVEL", "info")
			if flag := command.Flags().Lookup("log-level"); flag != nil {
				logLevel = flag.Value.String()
			} else if flag := command.InheritedFlags().Lookup("log-level"); flag != nil {
				logLevel = flag.Value.String()
			}
			requestURL, err := askURL(serviceURL)
			if err != nil {
				return err
			}
			payload, err := json.Marshal(deckyAskRequest{
				Game:     deckyGame{Source: "local", GameID: args[0], DisplayName: displayName},
				Question: args[1],
				Options:  deckyOptions{NoSpoilers: noSpoilers, AnswerLength: answerLength, NoRAG: noRAG},
			})
			if err != nil {
				return fmt.Errorf("encode Decky question: %w", err)
			}

			request, err := http.NewRequestWithContext(command.Context(), http.MethodPost, requestURL, bytes.NewReader(payload))
			if err != nil {
				return fmt.Errorf("create request: %w", err)
			}
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Accept", "text/event-stream, application/json")
			if strings.EqualFold(logLevel, "debug") {
				request.Header.Set("X-Walkthrough-Debug", "true")
			}
			clientCopy := *client
			clientCopy.Timeout = timeout
			response, err := clientCopy.Do(request)
			if err != nil {
				return fmt.Errorf("send question: %w", err)
			}
			defer response.Body.Close()
			if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
				message, _ := io.ReadAll(response.Body)
				return fmt.Errorf("ask service: %s: %s", response.Status, strings.TrimSpace(string(message)))
			}
			_, err = io.Copy(command.OutOrStdout(), response.Body)
			return err
		},
	}

	flags := command.Flags()
	flags.StringVar(&serviceURL, "service-url", environmentOrDefault("WALKTHROUGH_SERVICE_URL", defaultServiceURL), "local service base URL")
	flags.StringVar(&displayName, "display-name", "", "game name shown to the user")
	flags.BoolVar(&noSpoilers, "no-spoilers", true, "request an answer without spoilers")
	flags.BoolVar(&noRAG, "no-rag", false, "answer without retrieving walkthrough context")
	flags.StringVar(&answerLength, "answer-length", "short", "requested answer length")
	flags.DurationVar(&timeout, "timeout", 10*time.Minute, "service request timeout")
	return command
}

func askURL(serviceURL string) (string, error) {
	base, err := url.Parse(serviceURL)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return "", fmt.Errorf("parse service URL %q", serviceURL)
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/ask"
	return base.String(), nil
}
