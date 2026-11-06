package parser

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/walkthrough-assistance/backend/ingestion"
)

type CommandParser struct {
	command string
	args    []string
}

func NewCommand(command string, args ...string) *CommandParser {
	return &CommandParser{command: command, args: append([]string(nil), args...)}
}

func (p *CommandParser) Parse(ctx context.Context, raw ingestion.RawDocument) (ingestion.ParsedDocument, error) {
	if len(raw.Content) == 0 {
		return ingestion.ParsedDocument{}, ingestion.ErrEmptyDocument
	}
	if strings.HasPrefix(strings.ToLower(raw.ContentType), "text/plain") {
		markdown := strings.TrimSpace(strings.ReplaceAll(strings.ToValidUTF8(string(raw.Content), "\uFFFD"), "\r\n", "\n"))
		if markdown == "" {
			return ingestion.ParsedDocument{}, ingestion.ErrEmptyDocument
		}
		return ingestion.ParsedDocument{Markdown: markdown}, nil
	}
	if p.command == "" {
		return ingestion.ParsedDocument{}, errors.New("parser command is not configured")
	}

	command := exec.CommandContext(ctx, p.command, p.args...)
	command.Stdin = bytes.NewReader(raw.Content)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message != "" {
			return ingestion.ParsedDocument{}, fmt.Errorf("parser process: %w: %s", err, message)
		}
		return ingestion.ParsedDocument{}, fmt.Errorf("parser process: %w", err)
	}

	var parsed ingestion.ParsedDocument
	decoder := json.NewDecoder(&stdout)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&parsed); err != nil {
		return ingestion.ParsedDocument{}, fmt.Errorf("decode parser output: %w", err)
	}
	parsed.Title = strings.TrimSpace(parsed.Title)
	parsed.Markdown = strings.TrimSpace(strings.ReplaceAll(parsed.Markdown, "\r\n", "\n"))
	if parsed.Markdown == "" {
		return ingestion.ParsedDocument{}, ingestion.ErrEmptyDocument
	}
	return parsed, nil
}
